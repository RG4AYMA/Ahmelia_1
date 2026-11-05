package core

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"ahmelia/integration"
	"ahmelia/interaction"
	"ahmelia/personality"
	"ahmelia/situation"
)

// P4 — Posterior de preferencias (personality.PreferenceModel) al servicio del
// usuario (familia I3). Mecanismo que EXTIENDE (R12 declarado):
//
//  1. Alimenta Observe() SOLO con preferencias explícitas YA ACEPTADAS por el
//     camino canónico B96.34 (ApplyUserObservation → RunUserModelUpdateE2E) en
//     persistUserObservations / persistPersonalAnswerObservation. No duplica
//     la autoridad del UserModel: es una proyección derivada con olvido.
//  2. Su lectura entra como CONTRIBUCIÓN del modelo compartido de situación
//     (core/situation_convergence.go — patrón CompetencePerspective): el
//     posterior formado se traduce a Relations tipadas que situation.Evaluate
//     convierte en DecisionFindings y las propuestas leen. Nunca por el canal
//     AuthoritativeEvidence: es stance derivado, no afirmación canónica.
//  3. Persistencia bajo el contrato B90.7: carga honesta al arrancar
//     (fail-open a posterior vacío) y guardado tras cada aceptación.
//
// LIMITE DECLARADO: el mapeo captura la familia explícita POSITIVA de
// preferencias (favorito/me gusta) con guarda de polaridad falla cerrada
// (interaction.ExplicitPositivePreference): negaciones ligadas al gusto
// ("ya no me gusta…", "odio…") NO se convierten en Outcome positivo ni se
// inventan — requieren su propio camino de admisión antes de entrar al
// posterior. Su proyección es EPISTÉMICAMENTE DERIVADA: ObservedAt es la
// marca de la última observación del posterior (View.At), no time.Now().
// Sin evidencia la respuesta de View es "unformed" — el piso garantizado es
// la conducta de hoy sin cambios.

// UserPreferencesPath es la persistencia del posterior (no del modelo
// canónico, que sigue siendo memory/user_model.json de B96.34).
const UserPreferencesPath = "memory/user_preferences.json"

// loadUserPreferenceModel construye el posterior y restaura el snapshot si
// existe y es legible; ante archivo ausente o corrupto arranca limpio y lo
// declara (honestidad de arranque, patrón B90.7/T4 — nunca inventa datos).
func loadUserPreferenceModel() *personality.PreferenceModel {
	m := personality.NewPreferenceModel()
	raw, err := os.ReadFile(UserPreferencesPath)
	if err != nil {
		return m
	}
	snap := map[string]personality.Posterior{}
	if jsonErr := json.Unmarshal(raw, &snap); jsonErr != nil {
		fmt.Printf("⚠️  [P4] snapshot de preferencias ilegible (%s) - se arranca con posterior vacío\n", jsonErr)
		return m
	}
	if restoreErr := m.Restore(snap); restoreErr != nil {
		fmt.Printf("⚠️  [P4] snapshot de preferencias parcial: %v\n", restoreErr)
	}
	return m
}

// normalizePreferenceKey deriva la llave estable del posterior: el OBJETO de
// la preferencia. Recorte determinista del articulante de gusto ("me gusta",
// "prefiero", "mi favorito es" — la anteposición verbo-objeto del español),
// de artículos y espacios — mecanismo gramatical reutilizable, sin listas de
// frases de significado (R11/R12).
func normalizePreferenceKey(value string) string {
	v := strings.ToLower(strings.TrimSpace(strings.Join(strings.Fields(value), " ")))
	stripped := true
	for stripped && v != "" {
		stripped = false
		for _, art := range []string{
			"me gusta ", "me gustan ", "me encanta ", "me encantan ",
			"prefiero ", "mi favorito es ", "mi favorita es ",
		} {
			if strings.HasPrefix(v, art) {
				v = strings.TrimSpace(strings.TrimPrefix(v, art))
				stripped = true
			}
		}
	}
	for _, art := range []string{"el ", "la ", "los ", "las ", "un ", "una "} {
		v = strings.TrimPrefix(v, art)
	}
	for _, art := range []string{
		"el", "la", "los", "las", "un", "una",
		"me gusta", "me gustan", "me encanta", "me encantan",
		"prefiero", "mi favorito es", "mi favorita es",
	} {
		if v == art {
			return ""
		}
	}
	return strings.TrimSpace(v)
}

// observeAcceptedPreference registra en el posterior una preferencia explícita
// que el camino canónico B96.34 YA aceptó. Outcome +1 (declaración positiva de
// gusto por el objeto), Weight = confianza de admisión del propio registro.
// Guarda de POLARIDAD (Paso 2, falla cerrada): solo entra la familia positiva
// explícita — interaction.ExplicitPositivePreference rechaza negaciones
// ligadas al gusto ("ya no me gusta…", "odio…") y textos sin cabeza de gusto
// clara; lo rechazado no se registra ni se inventa (requiere su propio camino
// de admisión).
func (e *Engine) observeAcceptedPreference(obs interaction.UserObservation, now time.Time) {
	if e == nil || e.prefModel == nil || now.IsZero() {
		return
	}
	// La polaridad se evalúa sobre el ENUNCIADO completo que portó la
	// observación (valor + evidencia + contexto): el canal de respuesta
	// personal pasa texto libre como Value, donde la negación puede llegar.
	utterance := strings.Join([]string{obs.Value, obs.Evidence, obs.Context}, " ")
	if !interaction.ExplicitPositivePreference(utterance) {
		return
	}
	key := normalizePreferenceKey(obs.Value)
	if key == "" {
		return
	}
	weight := obs.Confidence
	if weight <= 0 || weight > 1 {
		weight = 0.8
	}
	ref := obs.Provenance.SourceID
	if ref == "" {
		ref = "conversation"
	}
	_ = e.prefModel.Observe(personality.Experience{
		Key:     key,
		Outcome: 1,
		Weight:  weight,
		Ref:     ref,
		At:      now,
	})
}

// saveUserPreferenceSnapshot persiste el posterior bajo el contrato B90.7
// ("guardado tras mutación"); falla de disco se declara y no bloquea el turno.
func (e *Engine) saveUserPreferenceSnapshot() {
	e.saveUserPreferenceSnapshotTo(UserPreferencesPath)
}

func (e *Engine) saveUserPreferenceSnapshotTo(path string) {
	if e == nil || e.prefModel == nil {
		return
	}
	snap := e.prefModel.Snapshot()
	if len(snap) == 0 {
		return
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		fmt.Printf("⚠️  [P4] snapshot de preferencias no serializable: %v\n", err)
		return
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		fmt.Printf("⚠️  [P4] snapshot de preferencias no guardado: %v\n", err)
		return
	}
}

// derivedPreferenceStances transmite SOLO los stances formados del posterior
// (los datos crudos viven en el modelo dueño; este es el ÚNICO punto en core
// que los traduce — R12: preferredEvidence y la contribución beben de acá).
func derivedPreferenceStances(m *personality.PreferenceModel) []integration.PreferenceStancePort {
	if m == nil {
		return nil
	}
	keys := m.Keys()
	out := make([]integration.PreferenceStancePort, 0, len(keys))
	for _, key := range keys {
		view := m.View(key)
		if view.Stance == personality.StanceUnformed {
			continue
		}
		out = append(out, integration.PreferenceStancePort{Key: key, Stance: string(view.Stance), Mean: view.Mean})
	}
	return out
}

// preferenceContribution construye la contribución de lectura del posterior
// formado (P4, directiva Paso 2 — mismo patrón que CompetencePerspective).
// Sin stances formados no hay contribución: la ausencia no se disimula.
// La contribución NO vota, no muta la situación y JAMÁS ejecuta o autoriza —
// solo hace legible al posterior en las propuestas (DecisionFindings).
// NOTA: la vía por AuthoritativeEvidence quedó RETIRADA (se descartaba en el
// conflict del turno); el único consumo es Contribution → situation.Evaluate
// → DecisionEvidence.Findings → propuesta interactiva.
func (e *Engine) preferenceContribution(sit situation.Situation) []situation.Contribution {
	formed := derivedPreferenceStances(e.prefModel)
	if len(formed) == 0 {
		return nil
	}
	return []situation.Contribution{integration.PreferencePerspective(sit, formed)}
}
