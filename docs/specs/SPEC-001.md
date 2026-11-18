Gestor de Tareas personales en consola

# SPEC-001: Gestor de Tareas Personales con Servidor MCP en Go

## 1. Resumen Ejecutivo

Construir un binario único en **Golang** (`taskflow`) que actúe como un gestor de tareas personales con interfaz de terminal interactiva (TUI) navegable por teclado y ratón, respaldado por SQLite, y que integre un **Servidor MCP (Model Context Protocol)** sobre `stdio` para que clientes/agentes de IA puedan descubrir, reclamar y resolver tareas delegables.

## 2. Arquitectura y Stack Tecnológico

* **Lenguaje:** Go 1.23+
* **Base de Datos:** SQLite (`modernc.org/sqlite` — driver puro en Go sin dependencias CGO para facilitar compilación cruzada) en modo `WAL` (`PRAGMA journal_mode=WAL;`) y `busy_timeout=5000`.
* **Interfaz de Usuario (TUI):** `charmbracelet/bubbletea` + `charmbracelet/lipgloss` + `charmbracelet/bubbles` (con soporte explícito de `tea.WithMouseCellMotion()`).
* **Servidor MCP:** `mark3labs/mcp-go` (o SDK oficial de MCP para Go) sobre transporte `stdio`.
* **Modos de Ejecución (CLI):**
* `taskflow` (sin argumentos): Inicia la TUI interactiva.
* `taskflow mcp`: Inicia el servidor MCP en modo `stdio` (silenciando logs hacia `stderr` o archivo para no corromper el canal JSON-RPC de `stdout`).



## 3. Modelo de Datos (SQLite)

### Tabla `tasks`

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

## 4. Especificación Funcional

### 4.1. Interfaz de Terminal (TUI con Soporte de Ratón)

La pantalla se dividirá en un tablero Kanban de 4 columnas o vista dividida (Lista + Panel de Detalle/Revisión):

1. **Columnas / Estados:**
* `PENDING`: Tareas nuevas o disponibles.
* `IN PROGRESS`: Tareas tomadas por un humano o reclamadas por un agente IA (muestra badge con `assigned_agent`).
* `REVIEW`: Tareas completadas por IA esperando validación humana.
* `DONE`: Tareas finalizadas.


2. **Interacciones con Ratón (`tea.MouseMsg`):**
* **Clic izquierdo en columna/tarea:** Selecciona la columna y enfoca la tarjeta de tarea correspondiente.
* **Doble clic en tarea:** Abre el modal de detalle completo (muestra `description` y `ai_notes`).
* **Rueda del ratón (Scroll Up/Down):** Desplaza la lista de tareas dentro de la columna bajo el cursor o el contenido del panel de detalle.
* **Clic en botones de acción rápida (barra inferior / cabecera):**
* `[+ Nueva Tarea]`
* `[Toggle IA]` (alterna `delegable_to_ai` entre `true`/`false`)
* `[Aprobar / Mover a Done]` (especialmente útil en estado `review`)
* `[Devolver a Pending]` (limpia `assigned_agent` para que otro agente pueda tomarla)




3. **Atajos de Teclado (Paridad con ratón):**
* `n`: Crear nueva tarea (formulario con Título, Descripción, Tags y checkbox Delegable a IA).
* `e`: Editar tarea seleccionada.
* `a`: Aprobar tarea en `review` -> pasa a `done`.
* `d`: Eliminar tarea (con confirmación).
* `r`: Recargar estado desde SQLite (y auto-refresh cada 2 segundos vía `tea.Tick` para ver cambios de agentes en tiempo real).
* `q` / `Ctrl+C`: Salir.



### 4.2. Servidor MCP (`taskflow mcp`)

#### Resources (Lectura de Contexto)

* **URI:** `tasks://pending-for-ai`
* **MIME Type:** `application/json`
* **Comportamiento:** Retorna todas las tareas donde `status = 'pending'` y `delegable_to_ai = 1`.



#### Tools (Acciones para Agentes)

1. **`list_available_tasks`**
* **Input:** `{ "tag_filter": "string (opcional)" }`
* **Comportamiento:** Consulta SQLite filtrando `status = 'pending' AND delegable_to_ai = 1` (y opcionalmente `tags LIKE ?`).
* **Output:** Lista JSON con `id`, `title`, `description`, `tags`, `created_at`.


2. **`claim_task`**
* **Input:** `{ "task_id": int, "agent_name": "string" }`
* **Comportamiento:** Ejecuta una actualización atómica:
`UPDATE tasks SET status = 'in_progress', assigned_agent = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'pending' AND delegable_to_ai = 1`
* **Regla de Concurrencia:** Si `RowsAffected == 0`, retorna error explícito (`isError: true`) indicando que la tarea no existe, no es delegable o ya fue reclamada por otro agente.


3. **`submit_task_result`**
* **Input:** `{ "task_id": int, "ai_notes": "string", "requires_review": bool }`
* **Comportamiento:** Actualiza `ai_notes` y cambia `status` a `'review'` (si `requires_review == true`) o `'done'` (si `requires_review == false`). Solo permite actualizar tareas cuyo estado actual sea `'in_progress'`.


4. **`create_subtask`**
* **Input:** `{ "title": "string", "description": "string", "delegable_to_ai": bool, "tags": ["string"] }`
* **Comportamiento:** Inserta una nueva tarea en estado `'pending'` y devuelve el `id` creado.



## 5. Estructura de Directorios Sugerida

```text
taskflow/
├── cmd/
│   └── taskflow/
│       └── main.go           # Entrypoint CLI (TUI vs subcomando mcp)
├── internal/
│   ├── domain/
│   │   └── task.go           # Struct Task, enums de Status y validaciones
│   ├── repository/
│   │   ├── sqlite.go         # Conexión WAL, migraciones iniciales
│   │   └── task_repo.go      # CRUD + Atomic Claim
│   ├── mcp/
│   │   └── server.go         # Definición de Resources y Tools MCP
│   └── tui/
│       ├── model.go          # Estado principal de Bubble Tea
│       ├── update.go         # Manejo de teclado, ratón (coordenadas/hitboxes) y ticks
│       ├── view.go           # Renderizado Lipgloss y tracking de zonas clicables
│       └── forms.go          # Modal de creación/edición de tareas
├── go.mod
└── go.sum

```

## 6. Criterios de Aceptación (Definition of Done)

* **CA-1 (Concurrencia SQLite):** Si dos procesos ejecutando `claim_task` intentan reclamar el mismo `task_id` simultáneamente, exactamente uno tiene éxito y el otro recibe un error sin bloqueos `SQLITE_BUSY`.
* **CA-2 (Integridad MCP):** Al configurar `{"command": "/ruta/a/taskflow", "args": ["mcp"]}` en un cliente MCP (ej. Claude Desktop o Cursor), el cliente lista las 4 tools y el resource sin errores de parseo JSON-RPC.
* **CA-3 (Interacción Ratón TUI):** El usuario puede seleccionar cualquier tarea con clic izquierdo, hacer scroll en listas largas con la rueda del ratón y aprobar una tarea en `review` haciendo clic en el botón de acción sin tocar el teclado.
* **CA-4 (Sincronización en Vivo):** Cuando un agente externo reclama o completa una tarea vía MCP mientras la TUI está abierta, la tarjeta se mueve automáticamente de columna en la TUI en menos de 3 segundos.

---
