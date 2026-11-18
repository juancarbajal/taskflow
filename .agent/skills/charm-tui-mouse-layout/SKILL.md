---
name: charm-tui-mouse-layout
description: >-
  Use this skill when building, modifying, styling, or debugging the terminal user
  interface (TUI) in internal/tui/ using Charmbracelet Bubble Tea, Lipgloss, and
  Bubbles. Activated for Kanban board rendering, mouse click/scroll hitboxes,
  action buttons, modals/forms, keyboard shortcuts, and real-time tea.Tick polling.
---

# Charm TUI, Mouse Hitboxes & Layout (`taskflow`)

## 1. Inicialización del Programa Bubble Tea (SPEC-001 §2, §4.1)

- Inicia siempre el programa con soporte de celdas de ratón y pantalla alternativa:
  ```go
  p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
  ```
- Nunca escribas con `fmt.Println` o `log.Print` a `stdout` mientras la TUI esté activa para no corromper el renderizado de `lipgloss`.

## 2. Layout del Tablero Kanban y Estados (SPEC-001 §4.1.1)

El tablero principal en `internal/tui/view.go` se organiza en 4 columnas (o vista dividida adaptable según `tea.WindowSizeMsg`):

1. **`PENDING`** (`pending`): Tareas nuevas o disponibles (muestra indicador si `delegable_to_ai == true`).
2. **`IN PROGRESS`** (`in_progress`): Tareas tomadas por humano o reclamadas por agente IA (debe renderizar un badge visible con `assigned_agent` cuando no esté vacío).
3. **`REVIEW`** (`review`): Tareas completadas por IA esperando validación humana.
4. **`DONE`** (`done`): Tareas finalizadas.

## 3. Interacciones con Ratón (`tea.MouseMsg`) y Hitboxes (SPEC-001 §4.1.2, CA-3)

En `internal/tui/update.go` y `internal/tui/view.go`, mantén un registro de zonas clicables (bounding boxes `{XMin, XMax, YMin, YMax}`) calculadas en cada `WindowSizeMsg` y actualización de layout:

1. **Clic izquierdo en columna o tarjeta (`tea.MouseActionPress`, `tea.MouseButtonLeft`)**:
   - Selecciona la columna bajo `(msg.X, msg.Y)` y enfoca la tarjeta de tarea correspondiente.
2. **Doble clic izquierdo en tarea**:
   - Detecta dos clics izquierdos consecutivos sobre el mismo `task.ID` dentro de una ventana de `~350ms`.
   - Abre el modal de detalle completo mostrando `description`, `tags`, `assigned_agent` y `ai_notes`.
3. **Rueda del ratón (`tea.MouseButtonWheelUp` / `tea.MouseButtonWheelDown`)**:
   - Si el modal de detalle está abierto, desplaza su viewport.
   - En el tablero, desplaza la lista de tareas de la columna situada bajo el cursor `(msg.X, msg.Y)` (o la columna activa).
4. **Botones de Acción Rápida Clicables (Barra inferior / Cabecera)**:
   - Renderiza botones con coordenadas registradas y responde al clic izquierdo directo sin requerir teclado:
     - `[+ Nueva Tarea]`: Abre el formulario modal de creación (`internal/tui/forms.go`).
     - `[Toggle IA]`: Alterna `delegable_to_ai` (`true`/`false`) en la tarea seleccionada.
     - `[Aprobar / Mover a Done]`: Mueve la tarea seleccionada (especialmente desde `review`) al estado `done`.
     - `[Devolver a Pending]`: Cambia `status` a `pending` y limpia `assigned_agent = ""` para que otro agente pueda tomarla.

## 4. Paridad con Atajos de Teclado (SPEC-001 §4.1.3)

Cuando ningún input de texto/formulario tenga el foco de escritura, garantiza paridad completa por teclado:

- `n`: Abrir formulario modal de nueva tarea (campos: Título, Descripción, Tags separados por coma, y toggle/checkbox Delegable a IA).
- `e`: Abrir formulario modal para editar la tarea seleccionada.
- `a`: Aprobar tarea en estado `review` -> pasa a `done`.
- `d`: Eliminar tarea seleccionada (mostrando confirmación previa).
- `r`: Recargar inmediatamente el estado desde SQLite.
- `q` / `Ctrl+C`: Salir limpiamente de la aplicación (`tea.Quit`).

## 5. Sincronización en Vivo cada 2 Segundos (SPEC-001 §4.1.3, CA-4)

- Implementa un `tea.Tick(2*time.Second, ...)` continuo desde `Init()` y re-encolado al recibir el mensaje de tick en `Update()`.
- Cada tick consulta SQLite de forma asíncrona (`tea.Cmd`) y actualiza las listas de tareas preservando el ID de la tarea seleccionada y la posición de scroll cuando sea posible.
- Garantiza que cuando un agente externo reclama o completa una tarea vía MCP, la tarjeta se mueva de columna en menos de 3 segundos (CA-4).
