---
name: sqlite-concurrency-go
description: >-
  Use this skill when implementing, modifying, or testing the SQLite persistence
  layer in internal/repository/ (sqlite.go, task_repo.go). Activated for database
  initialization, modernc.org/sqlite configuration, WAL mode, busy_timeout, schema
  migrations, JSON tags serialization, and atomic concurrent operations like
  claim_task.
---

# SQLite Concurrency & Persistence in Go (`taskflow`)

## 1. Driver y Configuración de Conexión (SPEC-001 §2, CA-1)

- **Driver obligatorio**: `modernc.org/sqlite` (driver SQLite puro en Go sin CGO, registrado para `database/sql` como `"sqlite"`).
- **PRAGMAs obligatorios al abrir la conexión**:
  Tanto el proceso de la TUI (`taskflow`) como uno o múltiples procesos `taskflow mcp` accederán simultáneamente al mismo archivo SQLite. Para evitar errores `SQLITE_BUSY` (`database is locked`):
  1. `PRAGMA journal_mode = WAL;`
  2. `PRAGMA busy_timeout = 5000;`
  3. `PRAGMA synchronous = NORMAL;`
  4. `PRAGMA foreign_keys = ON;`
- Aplica los pragmas en el DSN o inmediatamente tras `sql.Open("sqlite", dsn)` y verifica su activación con `db.ExecContext`.

## 2. Esquema e Índices Exactos (SPEC-001 §3)

Ejecuta la migración idempotente al inicializar el repositorio en `internal/repository/sqlite.go`:

```sql
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending', -- pending | in_progress | review | done
    delegable_to_ai BOOLEAN NOT NULL DEFAULT 1,
    tags TEXT DEFAULT '[]', -- Array JSON serializado (ej. ["go", "docs"])
    assigned_agent TEXT DEFAULT '',
    ai_notes TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_ai ON tasks(status, delegable_to_ai);
```

## 3. Operaciones Atómicas y Reglas de Concurrencia (SPEC-001 §4.2, CA-1)

### 3.1. Prohibición de Check-Then-Act en `ClaimTask`
- **NUNCA** hagas un `SELECT status FROM tasks WHERE id = ?` seguido de un `UPDATE` separado en Go para reclamar una tarea; eso introduce una condición de carrera entre agentes concurrentes.
- **SIEMPRE** ejecuta una única sentencia `UPDATE` atómica con las precondiciones en la cláusula `WHERE`:
  ```sql
  UPDATE tasks
  SET status = 'in_progress',
      assigned_agent = ?,
      updated_at = CURRENT_TIMESTAMP
  WHERE id = ?
    AND status = 'pending'
    AND delegable_to_ai = 1;
  ```
- Inspecciona `res.RowsAffected()`:
  - Si `rows == 1`: reclamo exitoso.
  - Si `rows == 0`: devuelve un error de dominio tipado (`domain.ErrTaskClaimFailed` / `ErrTaskNotAvailable`) para que la capa MCP retorne `isError: true`.

### 3.2. Transición Atómica en `SubmitTaskResult`
- Ejecuta un único `UPDATE` condicionado a `status = 'in_progress'`:
  ```sql
  UPDATE tasks
  SET status = ?,
      ai_notes = ?,
      updated_at = CURRENT_TIMESTAMP
  WHERE id = ?
    AND status = 'in_progress';
  ```
- Si `RowsAffected() == 0`, retorna error indicando que la tarea no existe o no está en estado `'in_progress'`.

### 3.3. Serialización de `tags`
- Al guardar en SQLite: serializa `[]string` a JSON (`json.Marshal`). Si el slice es `nil` o vacío, almacena `"[]"`.
- Al leer de SQLite: deserializa la columna `tags` (`json.Unmarshal`) hacia `[]string`. Si la cadena está vacía, inicializa como `[]string{}`.

## 4. Verificación de Concurrencia (CA-1)

Siempre incluye un test de concurrencia en `internal/repository/task_repo_test.go` que lance al menos 2 goroutines/conexiones intentando ejecutar `ClaimTask` sobre el mismo `task_id` simultáneamente (usando una barrera `sync.WaitGroup` o canal de cierre):
- Exactamente 1 llamada debe tener éxito (`err == nil`).
- Todas las demás llamadas deben fallar con el error de reclamo esperado.
- Ninguna llamada debe fallar con `SQLITE_BUSY` (`database is locked`).
