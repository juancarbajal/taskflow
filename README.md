# Taskflow 📋🤖

> **El tablero Kanban personal para terminal donde humanos y agentes de Inteligencia Artificial colaboran en tiempo real sobre la misma base de datos local.**

---

## 🎯 Visión de Producto (Product Overview)

Bienvenido a **Taskflow**. Como desarrolladores y profesionales técnicos, a menudo nos encontramos en una encrucijada: queremos la velocidad y la concentración que brinda la terminal para gestionar nuestras tareas del día a día, pero al mismo tiempo queremos delegar trabajo repetitivo o de investigación a asistentes de IA como Claude, Cursor o scripts autónomos.

**Taskflow** resuelve este desafío combinando dos mundos en un único binario ultra liviano:
1. **Un Tablero Kanban interactivo en consola:** Con soporte total para teclado y ratón (clic, scroll, botones interactivos) para gestionar tu flujo personal.
2. **Un Servidor MCP (Model Context Protocol) integrado:** Tus agentes de IA pueden inspeccionar las tareas pendientes marcadas como "Delegables a IA", reclamarlas de forma atómica para resolverlas, crear subtareas y reportar resultados en tiempo real.

Tanto tú como tus agentes de IA trabajan sobre la misma base de datos SQLite local (`WAL` mode). Cualquier acción que realice un agente se refleja en tu pantalla en menos de 2 segundos.

---

## ✨ Características Principales

- 🖱️ **TUI Dinámica con Ratón y Teclado:** Clic en tarjetas, doble clic para ver el detalle y notas de la IA, scroll con la rueda del ratón y botones de acción rápida.
- 🔄 **Sincronización en Vivo:** Polling continuo a SQLite; si una IA reclama una tarea o la envía a revisión, la tarjeta se mueve sola entre columnas en tiempo real.
- 🤖 **Diseñado para Delegar en IA:** Atributo `delegable_to_ai` por tarea y campo `ai_notes` para que la IA documente lo realizado.
- ⚡ **Cero Dependencias Externas (CGO-free):** Compilado con driver SQLite nativo en Go puro (`modernc.org/sqlite`), garantizando portabilidad y velocidad.
- 🔌 **Compatibilidad Nativa con MCP:** Expone Resources y Tools estándar vía `stdio` JSON-RPC 2.0.

---

## 🚀 Inicio Rápido para el Usuario

### 1. Compilación / Instalación

Asegúrate de contar con Go 1.23 o superior:

```bash
# Compilar el binario único
go build -o taskflow ./cmd/taskflow

# (Opcional) Mover al PATH del sistema
sudo mv taskflow /usr/local/bin/
```

### 2. Uso de la Interfaz Interactiva (TUI)

Para abrir tu tablero Kanban interactivo, ejecuta:

```bash
taskflow
```

*(Opcional: puedes indicar una base de datos específica con `taskflow --db /ruta/mi-tablero.db` o la variable de entorno `export TASKFLOW_DB=/ruta/mi-tablero.db`).*

#### 🎮 Controles y Navegación

| Acción | Ratón | Teclado |
| :--- | :--- | :--- |
| **Seleccionar Tarea / Columna** | Clic izquierdo sobre tarjeta o columna | Flechas direccionales (`←`, `→`, `↑`, `↓`) o `h`, `j`, `k`, `l` |
| **Ver Detalles / Notas de IA** | Doble clic en tarjeta | `Enter` |
| **Desplazar listas largas** | Rueda del ratón (Scroll Up / Down) | Flechas (`↑`, `↓`) / Viewport |
| **Nueva Tarea** | Clic en `[+ Nueva Tarea]` | `n` |
| **Editar Tarea** | — | `e` (permite editar título, descripción, tags y estado) |
| **Mover a "En Proceso"** | Clic en `[En Proceso]` | `2` o `]` (avanzar columna) |
| **Aprobar / Mover a "Done"** | Clic en `[Aprobar / Mover a Done]` | `a` o `4` |
| **Devolver a "Pending"** | Clic en `[Devolver a Pending]` | `p` o `1` |
| **Alternar Delegable a IA** | Clic en `[Toggle IA]` | `t` |
| **Eliminar Tarea** | — | `d` (solicita confirmación) |
| **Sincronizar Manualmente** | — | `r` (se sincroniza solo cada 2s) |
| **Salir** | — | `q` o `Ctrl+C` |

---

## 🤖 Integración con Clientes de Inteligencia Artificial (MCP)

Taskflow incluye un servidor **MCP (Model Context Protocol)** que opera sobre `stdio`. Cualquier herramienta compatible con MCP (Claude Desktop, Cursor, extensiones de VS Code, Goose, etc.) puede interactuar de forma segura con tu gestor de tareas.

### Modo Servidor MCP

El subcomando `taskflow mcp` inicia el transporte JSON-RPC silenciando cualquier salida no perteneciente al protocolo:

```bash
taskflow mcp --db /ruta/absoluta/a/taskflow.db
```

### Configuración en Claude Desktop

Agrega lo siguiente en tu archivo de configuración de Claude Desktop (`claude_desktop_config.json`):

- **Linux:** `~/.config/Claude/claude_desktop_config.json`
- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "taskflow": {
      "command": "/ruta/absoluta/a/taskflow",
      "args": ["mcp", "--db", "/ruta/absoluta/a/taskflow.db"]
    }
  }
}
```

### Configuración en Cursor / Otros Clientes MCP

En los ajustes de tu cliente de IA agrega el servidor:
- **Type / Transport:** `command` / `stdio`
- **Command:** `/ruta/absoluta/a/taskflow`
- **Args:** `["mcp", "--db", "/ruta/absoluta/a/taskflow.db"]`

---

## 🛠️ Capacidades Expuestas a los Agentes IA

Cuando conectas Taskflow a un modelo de IA, este adquiere las siguientes capacidades controladas:

### 📖 Recursos de Lectura (Resources)
- **`tasks://pending-for-ai`**: Permite al modelo leer en formato JSON todas las tareas actualmente en estado `pending` marcadas como `delegable_to_ai = 1`.

### ⚙️ Herramientas de Ejecución (Tools)

1. **`list_available_tasks`**
   - **Propósito:** Lista tareas delegables a IA, con opción de filtrar por etiqueta temática.
   - **Parámetros:**
     - `tag_filter` *(string, opcional)*: Ej. `"frontend"`, `"docs"`, `"go"`.

2. **`claim_task`**
   - **Propósito:** Reclama de forma atómica una tarea disponible y la mueve a `IN PROGRESS`, asignando el nombre del agente.
   - **Concurrencia:** Si dos agentes intentan reclamar la misma tarea a la vez, SQLite garantiza que solo uno tiene éxito y el otro recibe error sin bloqueos ni colisiones.
   - **Parámetros:**
     - `task_id` *(number, requerido)*: ID numérico de la tarea.
     - `agent_name` *(string, requerido)*: Identificador del agente (ej. `"Claude-Code"`).

3. **`submit_task_result`**
   - **Propósito:** Informa la resolución del trabajo, guarda las notas en `ai_notes` y transiciona la tarea a `REVIEW` (para tu aprobación humana) o `DONE`.
   - **Parámetros:**
     - `task_id` *(number, requerido)*: ID de la tarea que estaba en progreso.
     - `ai_notes` *(string, requerido)*: Explicación de los cambios realizados o enlaces a commits/archivos.
     - `requires_review` *(boolean, requerido)*: `true` para pasar a `REVIEW` y esperar el visto bueno humano; `false` para marcar como `DONE` directamente.

4. **`create_subtask`**
   - **Propósito:** Permite que un agente divida un requerimiento creando nuevas tareas dependientes en estado `pending`.
   - **Parámetros:**
     - `title` *(string, requerido)*
     - `description` *(string, opcional)*
     - `delegable_to_ai` *(boolean, requerido)*
     - `tags` *(array de strings, opcional)*

---

## 📁 Arquitectura y Desarrollo

El proyecto sigue una arquitectura limpia (**Clean Architecture**) en Go:

```text
.
├── cmd/
│   └── taskflow/
│       └── main.go           # CLI Router (TUI vs MCP)
├── internal/
│   ├── domain/               # Entidades, estados y validaciones puras
│   ├── repository/           # SQLite (WAL, busy_timeout=5000, atomic updates)
│   ├── mcp/                  # Servidor JSON-RPC 2.0 y tools MCP
│   └── tui/                  # Bubble Tea, Lipgloss y gestión de eventos de ratón
├── docs/
│   ├── specs/                # Especificaciones funcionales (SPEC-001, SPEC-002)
│   ├── arqs/                 # Decisiones arquitectónicas (ARCH-001, ARCH-002)
│   └── plans/                # Planes de ejecución y checklists (PLAN-001, PLAN-002)
└── go.mod
```

### Ejecutar Tests y Verificación de Concurrencia

```bash
# Ejecutar todas las pruebas con detector de carreras (race detector)
go test -v -race ./...
```

---

## 📄 Licencia

Código abierto bajo la licencia MIT. ¡Siéntete libre de adaptarlo y usarlo para potenciar tus flujos de trabajo con IA!
