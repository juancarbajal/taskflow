---
name: mcp-server-go-stdio
description: >-
  Use this skill when implementing, modifying, or testing the Model Context
  Protocol (MCP) server in internal/mcp/server.go or the `taskflow mcp` CLI
  subcommand. Activated for stdio JSON-RPC transport, MCP resources
  (tasks://pending-for-ai), and MCP tools (list_available_tasks, claim_task,
  submit_task_result, create_subtask).
---

# MCP Server over `stdio` in Go (`taskflow mcp`)

## 1. Regla de Oro del Transporte `stdio` (SPEC-001 §2, CA-2)

El subcomando `taskflow mcp` se comunica con clientes MCP (Claude Desktop, Cursor, agentes CLI) mediante JSON-RPC 2.0 sobre `os.Stdin` y `os.Stdout`.

- **PROHIBIDO escribir fuera del protocolo en `stdout`**: Ни `fmt.Print*`, ni `log.Print*` por defecto, ni banners de inicio pueden ir a `os.Stdout`.
- Al iniciar `taskflow mcp`, configura inmediatamente el logger estándar hacia `os.Stderr` (o archivo):
  ```go
  log.SetOutput(os.Stderr)
  ```
- Usa `mark3labs/mcp-go` (`server.NewMCPServer` + `server.ServeStdio`) o el SDK oficial de MCP para Go.

## 2. Especificación Exacta de Resources (SPEC-001 §4.2)

Registra el recurso de contexto para agentes:

- **URI**: `tasks://pending-for-ai`
- **MIME Type**: `application/json`
- **Comportamiento**: Consulta SQLite y retorna un array JSON con todas las tareas donde `status = 'pending'` y `delegable_to_ai = 1`. Si no hay tareas, retorna `[]` (nunca `null`).

## 3. Especificación Exacta de las 4 Tools (SPEC-001 §4.2, CA-2)

El servidor debe exponer exactamente estas 4 herramientas con sus esquemas de entrada y reglas de negocio:

### 3.1. `list_available_tasks`
- **Input Schema**:
  - `tag_filter` (`string`, opcional): Etiqueta para filtrar tareas.
- **Comportamiento**:
  - Consulta tareas con `status = 'pending' AND delegable_to_ai = 1`.
  - Si `tag_filter` no está vacío, filtra además las tareas cuyo array JSON `tags` contenga la etiqueta solicitada (`tags LIKE ?` o coincidencia de elemento JSON).
- **Output**: Lista JSON serializada con los campos `id`, `title`, `description`, `tags`, `created_at`.

### 3.2. `claim_task`
- **Input Schema**:
  - `task_id` (`number`/`int`, requerido)
  - `agent_name` (`string`, requerido, no vacío)
- **Comportamiento**:
  - Invoca la actualización atómica en SQLite:
    ```sql
    UPDATE tasks
    SET status = 'in_progress', assigned_agent = ?, updated_at = CURRENT_TIMESTAMP
    WHERE id = ? AND status = 'pending' AND delegable_to_ai = 1
    ```
- **Manejo de Concurrencia y Errores**:
  - Si `RowsAffected == 0`, **debe** retornar un resultado de herramienta con `isError: true` (`mcp.NewToolResultError(...)`) indicando explícitamente que la tarea no existe, no es delegable a IA o ya fue reclamada por otro agente.

### 3.3. `submit_task_result`
- **Input Schema**:
  - `task_id` (`number`/`int`, requerido)
  - `ai_notes` (`string`, requerido)
  - `requires_review` (`boolean`, requerido)
- **Comportamiento**:
  - Determina el nuevo estado: `'review'` si `requires_review == true`, o `'done'` si `requires_review == false`.
  - Actualiza `ai_notes`, `status` y `updated_at = CURRENT_TIMESTAMP` **únicamente** si el estado actual de la tarea en SQLite es `'in_progress'`.
  - Si la tarea no existe o su estado actual no es `'in_progress'` (`RowsAffected == 0`), retorna `isError: true`.

### 3.4. `create_subtask`
- **Input Schema**:
  - `title` (`string`, requerido, no vacío)
  - `description` (`string`, opcional)
  - `delegable_to_ai` (`boolean`, requerido)
  - `tags` (`array` de `string`, opcional)
- **Comportamiento**:
  - Inserta una nueva tarea en la tabla `tasks` con `status = 'pending'` y `tags` serializado como JSON array (por defecto `"[]"`).
  - Retorna un objeto JSON con el `id` recién creado.

## 4. Verificación Técnica (CA-2)

Prueba el servidor enviando un handshake JSON-RPC `initialize` y `tools/list` por `stdin`:
```bash
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}\n' | ./taskflow mcp
```
Verifica que la salida en `stdout` sea exclusivamente JSON-RPC válido y contenga las 4 tools.
