package actions

import (
	"fmt"

	"ahmelia/environment"
)

// ============================================================================
// ACTIONS/ — acciones específicas por aplicación ("compílalo", "ejecútalo")
// ============================================================================
//
// Por qué existe este paquete en vez de meter todo en core/engine.go:
// engine.go ya resuelve QUÉ app está activa (vía environment.Manager) y
// referencias implícitas genéricas (ábrelo, ciérralo, vuelve a eso — ver
// shared.IsPronoun). Pero "compílalo" no es genérico: compilar significa
// algo completamente distinto en Unity que en VSCode que en Blender. Meter
// eso en engine.go sería un switch creciente de "if unity... if vscode...
// if blender..." que se vuelve imposible de mantener según se agreguen
// apps. En cambio, cada app tiene su propio paquete (unity/, vscode/,
// blender/, unreal/) que implementa Handler, y engine.go solo le pregunta
// al Dispatcher "¿quién sabe manejar la app activa?" — no necesita saber
// nada de Unity ni de Blender.
//
// IMPORTANTE — este commit es SOLO el esqueleto. Los archivos dentro de
// unity/, vscode/, blender/, unreal/ existen y compilan, pero cada acción
// responde "todavía no implementado" en vez de hacer algo real. A
// propósito: no hay ninguna acción "compilar_programa" en el clasificador
// todavía, así que este paquete NO está conectado a core/engine.go — no
// tiene sentido cablear un comando de voz a una función que no hace nada
// real. Se conecta cuando la primera implementación real (Unity build,
// según se decida) esté lista.

// Handler es el contrato que cada plugin de app (unity, vscode, blender,
// unreal...) debe cumplir. Un Handler declara qué acciones entiende y
// cómo ejecutarlas para SU aplicación específica.
type Handler interface {
	// AppName es el nombre de proceso normalizado al que aplica este
	// handler (ej: "unity", "code", "blender") — la misma llave que usa
	// environment.Manager para identificar apps. El Dispatcher la usa
	// para decidir a quién preguntarle.
	AppName() string

	// Actions lista los verbos que este handler entiende, en español,
	// tal como se le dirían a Ahmelia (ej: "compilar", "reproducir").
	// Sirve tanto de documentación como para que el Dispatcher pueda
	// responder "esa app no sabe hacer eso" sin intentarlo primero.
	Actions() []string

	// Execute corre una acción concreta. Recibe el Manager completo (no
	// solo el nombre de la app) porque casi todas las acciones reales
	// necesitan Project/File de la app activa — ej: Unity necesita saber
	// QUÉ proyecto compilar, no solo que Unity está abierto.
	Execute(action string, world *environment.Manager) (string, error)
}

// Dispatcher enruta una acción ("compila", "ejecuta") al Handler de la
// app que esté activa ahora mismo, sin que el caller (core/engine.go)
// necesite saber qué apps existen ni cómo funciona cada una.
type Dispatcher struct {
	handlers map[string]Handler // key: Handler.AppName()
}

// NewDispatcher crea un Dispatcher vacío. Los handlers se registran
// aparte (ver Register) — normalmente desde core/engine.go al arrancar,
// igual que ya se hace con knowledge.Engine.Register para las fuentes de
// conocimiento.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]Handler)}
}

// Register agrega un Handler al Dispatcher. Si ya había uno registrado
// para la misma AppName, lo reemplaza — última palabra gana, igual de
// simple que el resto del sistema de registro de este proyecto.
func (d *Dispatcher) Register(h Handler) {
	d.handlers[h.AppName()] = h
}

// Dispatch resuelve la app activa (vía world.ActiveApp(), que ya
// reconcilia con el foco real de Windows) y le pasa la acción a su
// Handler. Dos formas honestas de fallar, distintas a propósito:
//   - no hay ninguna app activa que referenciar
//   - la app activa no tiene ningún Handler todavía (más común mientras
//     el esqueleto no tenga implementaciones reales)
func (d *Dispatcher) Dispatch(action string, world *environment.Manager) (string, error) {
	app, ok := world.ActiveApp()
	if !ok {
		return "", fmt.Errorf("no sé a qué aplicación te refieres, no tengo ninguna reciente")
	}

	h, ok := d.handlers[app.Name]
	if !ok {
		return "", fmt.Errorf("%s no tiene acciones específicas todavía", app.Name)
	}

	return h.Execute(action, world)
}
