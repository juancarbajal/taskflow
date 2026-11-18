# PLAN-002: Plan de Implementación para Gestión Manual de Estados

- **Especificación:** [`./docs/specs/SPEC-002.md`](../specs/SPEC-002.md)
- **Arquitectura:** [`./docs/arqs/ARCH-002.md`](../arqs/ARCH-002.md)

---

## 1. Tareas de Implementación

1. **T1 — Repositorio (`internal/repository/task_repo.go` & `task_repo_test.go`)**:
   - Implementar `MoveToStatus(ctx context.Context, taskID int64, status domain.Status, actor string) error`.
   - Agregar tests unitarios verificando transición a `in_progress` (asignando `"humano"` si no había agente), transición a `review` / `done` y retroceso a `pending` (limpiando `assigned_agent`).

2. **T2 — Modelo y Formularios TUI (`internal/tui/model.go` & `internal/tui/forms.go`)**:
   - Registrar `ActionInProgress` (`[En Proceso]`) en `buttonRects` dentro de `recalculateLayout()`.
   - Añadir `formStatus` y helpers de ciclo de estado (`nextStatus`, `prevStatus`) en `forms.go` para el modo edición.

3. **T3 — Controlador y Vista TUI (`internal/tui/update.go` & `internal/tui/view.go`)**:
   - Renderizar el botón `[En Proceso]` en la barra superior alineado con `buttonRects`.
   - Renderizar el selector de estado en `renderFormModal()` cuando `m.isEditForm == true`.
   - Manejar clic izquierdo en `[En Proceso]`, atajos `]`/`[`/`Shift+Right`/`Shift+Left`/`1`..`4` en `ModeBoard`, y navegación/selección de estado en `ModeForm`.
   - Actualizar la línea de ayuda inferior para mostrar los nuevos atajos (`[/]` mover columna, `1-4` estado).

4. **T4 — Pruebas TUI y Verificación Completa**:
   - Añadir test en `internal/tui/tui_test.go` cubriendo **CA-5** y **CA-6** (clic en `[En Proceso]`, atajos `]`/`[`/`1`..`4`, y cambio de estado desde formulario de edición).
   - Compilar `CGO_ENABLED=0 go build -o taskflow ./cmd/taskflow`.
   - Ejecutar `go vet ./...` y `go test -v -race ./...` verificando **CA-1** a **CA-7**.

---

## 2. Checklist de Aceptación

- [x] **CA-5:** Cambio manual a `IN PROGRESS` funcional por botón `[En Proceso]`, teclado y formulario de edición.
- [x] **CA-6:** Retroceso a `PENDING` limpia `assigned_agent` en SQLite.
- [x] **CA-7:** Todos los tests previos de `SPEC-001` (`CA-1` a `CA-4`) siguen pasando sin regresiones.
