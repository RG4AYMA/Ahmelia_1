# recon_T3 — Inventario de reconciliación: spec T3 vs árbol real

**Fecha:** 2026-10-16
**Autor:** GLM (OpenCode), tarea T3.1 según `docs/spec_T3.md` §0.
**Base auditada:** raíz del repositorio (`ahmelia`, go 1.23.2). Referencias: [[Auditoria_Arquitectura_Ahmelia_2026-10-16]] (nota del vault, 2026-10-16) y el tablero [[Coordinacion_entre_IAs_Ahmelia]].

## 0. Motivo de este documento

`spec_T3.md` fue escrito sin ver el repositorio ("No vi el repositorio", §0). La auditoría del 2026-10-16 verificó que **la mayoría de los pasos de T3 ya están implementados** en este árbol. Este documento es el reconocimiento obligatorio de T3.1: qué existe, qué falta y qué se desvía del spec, con evidencia `ruta:línea`. Regla R8 del spec se aplica invertida: no solo "si un símbolo no existe, detente" — también "si ya existe, no lo re-crees".

## 1. Inventario por paso

| Paso | Qué pide el spec | Estado en el árbol | Evidencia |
|---|---|---|---|
| T3.0 Verificación base | build/vet/tests subset `F9\|F10Invariant` | **PARCIAL — insuficiente.** `go build ./...` y `go vet ./...` limpios; 8 paquetes PASS. Pero la suite completa de `core` FALLA (9 tests, 3 familias). El subset del spec no los incluye: la aceptación "todo en verde" queda vacía si se corre solo ese filtro. | Log: `.tmp-gocache/core_suite_2026-10-16.log` (vault). Requiere T0 antes de cerrar T3. |
| T3.1 Reconocimiento | `docs/recon_T3.md` | **HECHO HOY** (este documento; `docs/` no existía). | Este archivo. |
| T3.2 Perfil de hardware | `resource/profile.go` + tipos + salida `memory/hardware_profile.json` | **EXISTE** con tests. La salida `memory/hardware_profile.json` NO existe: el perfil nunca se ejecutó. | `resource/profile.go`, `resource/profile_test.go`. |
| T3.3 Sonda Ollama | interfaz `Prober`, presupuesto VRAM | **EXISTE** con tests: `OllamaProber`, `VRAMBudgetMB` (default 1800 = GTX 1050 Ti). | `resource/probe.go`, `resource/probe_test.go`. |
| T3.4 Herramienta de perfilado | `cmd/ahmelia-profile/` | **EXISTE** (main.go + main_test.go) y hay binario compilado `ahmelia-profile.exe` (9.2 MB) en la raíz del repo. Diferencia menor: no hay `testdata/profile/prompts.json` en la carpeta del comando — verificar dónde cargan los prompts antes de asumir que falta. | `cmd/ahmelia-profile/main.go`, `cmd/ahmelia-profile/main_test.go`. |
| T3.5 Gobernador | `resource/governor.go`: ranura única, prioridades, anti-inanición, `Clock` inyectable | **EXISTE** con tests. | `resource/governor.go` ("ranura única de LLM con prioridades y anti-inanición", línea 58), `resource/governor_test.go`. |
| T3.6 Enrutado | `resource/router.go` por clase de tarea | **EXISTE** con tests. | `resource/router.go` (`RouterConfig`, línea 29), `resource/router_test.go`. |
| T3.7 Compuerta | campo opcional `Gate` en cliente de `linguistics` + adaptador | **EXISTE** el contrato: `type Gate interface` (`linguistics/ollama.go:63`) y el campo `Gate` (línea 109); adaptador `resource/gate.go`. **El cableado NO existe:** `core/engine.go` tiene cero referencias a Governor/Gate/Router (grep 2026-10-16). Sin cableado, la compuerta es nil y el comportamiento es el de siempre — correcto según spec, pero T3 no produce aún su efecto. | `linguistics/ollama.go`, `resource/gate.go`. |
| T3.8 Señal de trabajo | `resource/progress.go` | **EXISTE** con tests. Sin cablear (correcto: el spec lo cablea en T8). | `resource/progress.go`, `resource/progress_test.go`. |
| T3.9 Harness anti-escena | `variation/` + `testdata/forbidden_scene_literals.txt` | **EXISTE; brechas cerradas HOY por GLM** (ver §2). 14/14 tests PASS ×2 consecutivas. | `variation/variation.go`, `variation/audit_test.go`, `variation/testdata/forbidden_scene_literals.txt`. |
| T3.10 Cierre | perfil real con Ollama + `go test` de los 4 paquetes | **NO HECHO.** El propietario debe ejecutar `ahmelia-profile` con Ollama real. Nota: fijar `keep_alive` o pre-calentar el modelo; cargas frías documentadas de 23-26 s contaminarían el p95 (evidencia: nota del vault 2026-10-05). | `memory/hardware_profile.json` no existe. |

## 2. Desviaciones del spec encontradas y corregidas hoy (T3.9, GLM)

Corrida inicial: `go test ./variation -count=1 -v` = 12/12 PASS, pero con dos desviaciones de la aceptación y un test vacuo:

1. **`ForAll` no era reproducible** (aceptación "misma semilla, mismas situaciones" violada): usaba `time.Now().UnixNano()` y no existía forma de pasar la semilla. Corregido: semilla fija (`defaultForAllSeed`) + `ForAllSeed` explícita para replay. El reporte de fallo imprime semilla, plantilla, valores y texto de la situación mínima.
2. **`TestForAll_FailsAndReports` pasaba vacío**: la plantilla por defecto hardcodeaba slots `user_state/time_of_day/activity`; con otras dimensiones los placeholders quedaban sin resolver y la propiedad nunca fallaba. Corregido: `defaultTemplate` deriva un slot por cada dimensión provista. El test ahora usa un mock de `TestingT` y verifica el reporte como contrato.
3. **`SurfaceVariants` no era determinista** (rand global sin semilla). Corregido: sustituciones sistemáticas en orden fijo, sin aleatoriedad.
4. **`GeneratedSituation.Seed` consumía un sorteo extra del RNG** (no era la semilla base y perturbaba la secuencia). Corregido: `Seed` = semilla base del generador.
5. **`testdata/forbidden_scene_literals.txt`** estaba en `testdata/` de la raíz del repo; el spec lo pide en `variation/testdata/` y el loader prefería esa ruta primero. Movido a su lugar del spec. La lista la completa el propietario (frases completas de ejemplo de la visión; no palabras sueltas).

## 3. Paquetes de decisión auditados por `TestAudit_ForbiddenSceneLiterals`

`autonomy`, `initiative`, `situation`, `capability`, `planning`, `cognition`, `interaction`.

Justificación: son los paquetes donde reside decisión autónoma (qué hacer, cuándo, con qué autoridad y sobre qué). `core` es el motor que los orquesta pero contiene mayormente infraestructura de turno; `language`/`linguistics` son renderizado de respuesta (el contenido de la decisión ya está tomado); `resource` es gobernanza de recursos. Si el propietario quiere endurecer, añadir `core` a la lista — se espera que primero requiera limpiar literales de diagnóstico, no de escena.

## 4. Qué falta para cerrar T3 (en orden)

1. **T0 (DeepSeek 4.1v, en curso):** suite completa de `core` verde ×2, git init, hermeticidad. Sin esto, la aceptación de T3.0 "todo en verde" no mide nada real.
2. **Decisión de cableado (propietario):** el spec T3.7 dejó el campo `Gate` y el adaptador listos; conectar `Governor`/`Gate`/`Router` en la construcción del cliente del Engine es el único código que falta de T3 y toca `core/engine.go` — debe hacerse DESPUÉS de T0 para no pisar archivos.
3. **T3.10 (propietario):** ejecutar `ahmelia-profile` con Ollama real (con `keep_alive` fijado) y traer `memory/hardware_profile.json`.
4. Re-corrida de `go test ./resource ./variation ./cmd/ahmelia-profile ./linguistics` + subset de T3.0 como pide el spec.

## 5. Estado epistémico de este documento

- Existencias de archivos y símbolos: OBSERVADAS estáticamente con `ruta:línea` (2026-10-16).
- Corridas de test: OBSERVADAS en runtime de este árbol hoy (14/14 ×2 en `variation`; 8 paquetes PASS en auditoría de la mañana).
- "Cero referencias en engine.go": OBSERVADO por grep sobre `core/engine.go` a las 11:18 del 2026-10-16; puede cambiar mientras DeepSeek edita para T0 — reverificar al cablear.
- Ejecución del perfil real: NO observada (no hay evidencia); no afirmar que T3 está cerrado.
