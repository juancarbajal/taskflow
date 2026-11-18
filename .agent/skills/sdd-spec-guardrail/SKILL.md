---
name: sdd-spec-guardrail
description: >-
  Use this skill as a Spec-Driven Development (SDD) guardrail whenever planning,
  implementing, reviewing, or verifying features in the taskflow repository.
  Activated to validate strict compliance with SPEC-001.md data models, CLI modes,
  MCP contracts, TUI workflows, and Acceptance Criteria (CA-1 through CA-4).
---

# SDD Spec Guardrail (`SPEC-001.md`)

## 1. Propósito del Guardrail

Este proyecto sigue **Spec-Driven Development (SDD)** gobernado por [SPEC-001.md](../../../SPEC-001.md). Ninguna implementación debe desviarse de los contratos definidos en la especificación ni introducir dependencias o abstracciones fuera del alcance.

## 2. Matriz de Cumplimiento Obligatorio (`SPEC-001.md`)

Antes de dar por completada cualquier tarea o fase de implementación, verifica cada punto de esta lista de control:

### A. Binario y Modos CLI (§1, §2)
- [ ] El ejecutable se construye bajo `./cmd/taskflow` generando el binario `taskflow`.
- [ ] `taskflow` (sin argumentos) inicia la TUI interactiva con Bubble Tea (`tea.WithMouseCellMotion()`).
- [ ] `taskflow mcp` inicia el servidor MCP sobre `stdio` sin emitir ningún log o texto fuera de JSON-RPC en `stdout`.
- [ ] Driver SQLite puro en Go (`modernc.org/sqlite`) sin requerir CGO, con `PRAGMA journal_mode=WAL;` y `busy_timeout=5000`.

### B. Modelo de Datos SQLite (§3)
- [ ] Tabla `tasks` con las 10 columnas exactas: `id`, `title`, `description`, `status`, `delegable_to_ai`, `tags`, `assigned_agent`, `ai_notes`, `created_at`, `updated_at`.
- [ ] Valores de `status` restringidos a: `'pending'`, `'in_progress'`, `'review'`, `'done'`.
- [ ] Índice `idx_tasks_status_ai` creado sobre `tasks(status, delegable_to_ai)`.

### C. Interfaz TUI y Ratón (§4.1)
- [ ] 4 columnas de estado visibles: `PENDING`, `IN PROGRESS` (con badge de `assigned_agent`), `REVIEW`, `DONE`.
- [ ] Soporte de ratón (`tea.MouseMsg`):
  - Clic izquierdo selecciona columna y tarea.
  - Doble clic abre modal de detalle con `description` y `ai_notes`.
  - Rueda del ratón hace scroll en la columna o panel de detalle.
  - Botones clicables en barra: `[+ Nueva Tarea]`, `[Toggle IA]`, `[Aprobar / Mover a Done]`, `[Devolver a Pending]`.
- [ ] Atajos de teclado con paridad total: `n` (nueva), `e` (editar), `a` (aprobar `review` -> `done`), `d` (eliminar con confirmación), `r` (recargar + auto-refresh cada 2s vía `tea.Tick`), `q`/`Ctrl+C` (salir).

### D. Contratos del Servidor MCP (§4.2)
- [ ] Resource `tasks://pending-for-ai` (`application/json`) retorna tareas con `status = 'pending'` y `delegable_to_ai = 1`.
- [ ] Tool `list_available_tasks`: input opcional `tag_filter`; retorna `id`, `title`, `description`, `tags`, `created_at`.
- [ ] Tool `claim_task`: input `task_id`, `agent_name`; `UPDATE` atómico; retorna `isError: true` si `RowsAffected == 0`.
- [ ] Tool `submit_task_result`: input `task_id`, `ai_notes`, `requires_review`; cambia estado a `'review'` o `'done'` solo si estaba en `'in_progress'`.
- [ ] Tool `create_subtask`: input `title`, `description`, `delegable_to_ai`, `tags`; inserta en `'pending'` y devuelve el `id`.

## 3. Verificación de Criterios de Aceptación (Definition of Done - §6)

Toda entrega final debe poder demostrar el cumplimiento de los 4 Criterios de Aceptación:

1. **CA-1 (Concurrencia SQLite)**: Test automatizado donde procesos/goroutines concurrentes ejecutan `claim_task` sobre el mismo `task_id`; exactamente uno triunfa y el otro recibe error explícito sin `SQLITE_BUSY`.
2. **CA-2 (Integridad MCP)**: El subcomando `taskflow mcp` responde a `initialize`, `tools/list` y `resources/list` sobre `stdio` listando las 4 tools y el resource `tasks://pending-for-ai` con JSON-RPC limpio.
3. **CA-3 (Interacción Ratón TUI)**: Hitboxes funcionales para selección de tareas con clic izquierdo, scroll con rueda del ratón y clic en los 4 botones de acción rápida (incluido aprobar tarea en `review`).
4. **CA-4 (Sincronización en Vivo)**: El `tea.Tick` de 2 segundos refresca el estado desde SQLite moviendo las tarjetas reclamadas o completadas vía MCP en menos de 3 segundos.
