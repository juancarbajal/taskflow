# SPEC-002: Gestión Manual de Estados de Tareas en la TUI (`taskflow`)

- **Especificación Base:** [`SPEC-001.md`](./SPEC-001.md)
- **Alcance:** `internal/repository`, `internal/tui` (sin alterar contratos MCP de `SPEC-001`)

---

## 1. Problema y Objetivo

En [`SPEC-001.md`](./SPEC-001.md), la TUI únicamente exponía transiciones hacia `done` (`[Aprobar / Mover a Done]` / tecla `a`) y `pending` (`[Devolver a Pending]` / tecla `p`), asumiendo que el paso por `in_progress` y `review` ocurría solo mediante agentes MCP. Esto impide que un usuario humano mueva manualmente sus tareas desde la consola al estado `IN PROGRESS` o `REVIEW`.

El objetivo de esta especificación es habilitar la **gestión manual completa de estados desde la TUI** (por ratón, teclado y formulario de edición), respaldada por una operación dedicada en el repositorio SQLite.

---

## 2. Requisitos Funcionales

### 2.1. Repositorio (`internal/repository/task_repo.go`)
- Agregar el método `MoveToStatus(ctx context.Context, taskID int64, status domain.Status, actor string) error`:
  1. Valida que `status.IsValid()`; si no lo es, retorna `domain.ErrValidation`.
  2. Actualiza `status` y `updated_at = CURRENT_TIMESTAMP` en SQLite de forma atómica.
  3. **Reglas sobre `assigned_agent`:**
     - Si `status == domain.StatusPending`: limpia `assigned_agent = ''`.
     - Si `status == domain.StatusInProgress` y `assigned_agent == ''` y `actor != ""`: establece `assigned_agent = actor` (ej. `"humano"` cuando la mueve el usuario desde la TUI).
     - En otros casos, preserva `assigned_agent`.
  4. Si `RowsAffected() == 0`, retorna `domain.ErrTaskNotFound`.

### 2.2. Interacciones con Ratón en la TUI (`internal/tui`)
1. **Nuevo Botón de Acción Rápida `[En Proceso]`:**
   - Se añade en la barra superior de acciones rápidas con hitbox registrada (`ActionInProgress`), junto a `[+ Nueva Tarea]`, `[Toggle IA]`, `[En Proceso]`, `[Aprobar / Mover a Done]` y `[Devolver a Pending]`.
   - Al hacer clic izquierdo sobre `[En Proceso]`, mueve la tarea seleccionada al estado `in_progress` con `actor = "humano"`.
2. **Selector de Estado en Formulario de Edición (`e`):**
   - Cuando el formulario modal se abre en modo edición (`isEditForm == true`), muestra un selector cíclico de estado con los 4 valores (`pending`, `in_progress`, `review`, `done`) y su hitbox clicable.
   - El clic izquierdo sobre el selector avanza al siguiente estado en ciclo.

### 2.3. Atajos de Teclado en la TUI (`internal/tui`)
- **Movimiento secuencial entre columnas:**
  - `]` o `Shift+Right`: Avanza la tarea seleccionada a la siguiente columna (`pending` -> `in_progress` -> `review` -> `done`).
  - `[` o `Shift+Left`: Retrocede la tarea seleccionada a la columna anterior (`done` -> `review` -> `in_progress` -> `pending`).
- **Movimiento directo por número:**
  - `1`: Mover tarea seleccionada a `PENDING`.
  - `2`: Mover tarea seleccionada a `IN PROGRESS` (con `actor = "humano"` si no tiene agente).
  - `3`: Mover tarea seleccionada a `REVIEW`.
  - `4`: Mover tarea seleccionada a `DONE`.
- **Dentro del Formulario de Edición (`e`):**
  - El selector de estado forma parte del ciclo de foco (`Tab` / `Shift+Tab`) y cambia de estado con `Espacio`, `Left` o `Right`.

---

## 3. Criterios de Aceptación

- **CA-5 (Transición Manual a `IN PROGRESS`):** El usuario puede mover una tarea a `IN PROGRESS` desde la TUI usando el botón `[En Proceso]`, los atajos `]` / `Shift+Right` / `2`, o el selector de estado en el modal de edición (`e`), persistiendo el cambio en SQLite y moviendo la tarjeta de columna.
- **CA-6 (Retroceso y Limpieza de Agente):** Al mover una tarea hacia `PENDING` (con `[` / `Shift+Left` / `1` / `[Devolver a Pending]` / formulario), `assigned_agent` queda vacío (`""`) en SQLite.
- **CA-7 (No Regresión):** Todos los criterios de aceptación de `SPEC-001.md` (**CA-1** a **CA-4**) y los contratos del servidor MCP permanecen intactos y pasan `go test -race ./...`.
