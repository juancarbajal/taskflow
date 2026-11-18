---
name: golang-idiomatic-clean-arch
description: >-
  Use this skill whenever designing, writing, refactoring, or reviewing Go code,
  package boundaries, CLI entrypoints, domain models, or dependency injection in
  the taskflow project. Activated when working on cmd/taskflow or internal/
  packages (domain, repository, mcp, tui) using Go 1.23+.
---

# Golang Idiomatic & Clean Architecture (`taskflow`)

## 1. Estructura de Paquetes y Fronteras (SPEC-001 §5)

Respeta estrictamente el árbol de paquetes bajo arquitectura limpia y el aislamiento de `internal/`:

```text
taskflow/
├── cmd/
│   └── taskflow/
│       └── main.go           # Entrypoint CLI (TUI vs subcomando mcp)
├── internal/
│   ├── domain/
│   │   └── task.go           # Struct Task, enums de Status y validaciones puras
│   ├── repository/
│   │   ├── sqlite.go         # Conexión WAL, pragmas y migraciones iniciales
│   │   └── task_repo.go      # CRUD + Atomic Claim sobre SQLite
│   ├── mcp/
│   │   └── server.go         # Definición de Resources y Tools MCP
│   └── tui/
│       ├── model.go          # Estado principal de Bubble Tea
│       ├── update.go         # Manejo de teclado, ratón (coordenadas/hitboxes) y ticks
│       ├── view.go           # Renderizado Lipgloss y tracking de zonas clicables
│       └── forms.go          # Modal de creación/edición de tareas
```

### Reglas de Dependencia entre Capas
1. **`internal/domain` es el núcleo puro**:
   - No debe importar `database/sql`, `modernc.org/sqlite`, `charmbracelet/*` ni `mark3labs/mcp-go`.
   - Define `Task`, el tipo `Status` (`pending`, `in_progress`, `review`, `done`), errores de dominio (ej. `ErrTaskNotFound`, `ErrTaskAlreadyClaimed`, `ErrInvalidTransition`) y constructores/validadores de entidad.
2. **`internal/repository` depende únicamente de `internal/domain`**:
   - Implementa el acceso a SQLite y traduce filas SQL y serialización JSON de `tags` (`[]string` <-> `TEXT`) hacia/desde `domain.Task`.
3. **`internal/tui` e `internal/mcp` son adaptadores primarios**:
   - Ambos consumen `internal/domain` e interfaces/implementaciones de `internal/repository`.
   - Prohibido que `internal/tui` importe `internal/mcp` o viceversa.
4. **`cmd/taskflow/main.go` es el Composition Root**:
   - Parsea los argumentos CLI:
     - Sin argumentos (`taskflow`): Inicializa el repositorio SQLite y arranca el programa Bubble Tea (`internal/tui`).
     - Con subcomando `mcp` (`taskflow mcp`): Redirige o silencia cualquier log hacia `stderr`/archivo, inicializa el repositorio SQLite y lanza el servidor MCP sobre `stdio` (`internal/mcp`).

## 2. Reglas de Go 1.23+ Idiomático

- **Cero CGO**: No uses paquetes que requieran `CGO_ENABLED=1`. El binario único `taskflow` debe compilar de forma cruzada limpiamente con `CGO_ENABLED=0`.
- **Propagación de Contexto**: Todos los métodos de `repository` deben recibir `ctx context.Context` como primer parámetro (`ListAvailableTasks(ctx, tagFilter)`, `ClaimTask(ctx, id, agentName)`, etc.).
- **Manejo de Errores**:
  - Envuelve errores con contexto claro usando `fmt.Errorf("repository.ClaimTask: %w", err)`.
  - Usa `errors.Is` y `errors.As` en las capas `mcp` y `tui` para distinguir errores de negocio (ej. tarea ya reclamada) de fallos de infraestructura.
- **Modelado de `Status` y `Task`**:
  - Define constantes tipadas para estados válidos:
    - `StatusPending = "pending"`
    - `StatusInProgress = "in_progress"`
    - `StatusReview = "review"`
    - `StatusDone = "done"`
  - Valida siempre las transiciones de estado antes o durante la operación atómica (ej. `submit_task_result` solo aplica si el estado actual es `in_progress`; aprobar en TUI pasa de `review` a `done`).

## 3. Verificación Técnica

- Ejecuta `CGO_ENABLED=0 go build ./cmd/taskflow` para verificar que el binario compila sin dependencias C.
- Ejecuta `go vet ./...` y `go test -race ./...` para validar seguridad de tipos y ausencia de data races.
