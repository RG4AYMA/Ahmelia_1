package core

import "testing"

// TestIsBareConfirmation_SoloControlDeFlujo confirma que respuestas de
// puro control de flujo ("confirmar", "sí", "dale, adelante") se
// detectan como tal — son las que NO deben anexarse al objetivo
// original de un PendingClarification, porque no agregan ningún
// contenido sobre el dominio.
func TestIsBareConfirmation_SoloControlDeFlujo(t *testing.T) {
	casos := []string{"confirmar", "sí", "si", "dale", "adelante", "ok", "vale", "correcto", "dale, adelante"}
	for _, c := range casos {
		if !isBareConfirmation(c) {
			t.Errorf("isBareConfirmation(%q) = false, esperaba true", c)
		}
	}
}

// TestIsBareConfirmation_ConContenidoRealNoEsBareConfirmation confirma
// que una respuesta que trae contenido real, aunque empiece con una
// palabra de confirmación, NO se trata como control de flujo puro —
// ese contenido tiene que llegar al reanálisis fusionado.
func TestIsBareConfirmation_ConContenidoRealNoEsBareConfirmation(t *testing.T) {
	casos := []string{
		"sí, para modelado 3D y animaciones",
		"para hacer modelados 3d de animaciones y shaders",
		"quiero aprenderlo para videojuegos",
	}
	for _, c := range casos {
		if isBareConfirmation(c) {
			t.Errorf("isBareConfirmation(%q) = true, esperaba false (trae contenido real)", c)
		}
	}
}

// TestIsBareConfirmation_ConAuxiliarDeHaberSigueSiendoConfirmacion
// (Integration Fix) reproduce el bug real visto en producción: el
// reconocimiento de voz transcribió una confirmación como "ha
// confirmado", y como "ha" no se restaba, quedaba tratada como
// contenido real y terminaba pegada al MainGoal fusionado ("Blender
// ha confirmado"), contaminando la Skill guardada.
func TestIsBareConfirmation_ConAuxiliarDeHaberSigueSiendoConfirmacion(t *testing.T) {
	casos := []string{"ha confirmado", "he confirmado", "han confirmado", "hemos confirmado"}
	for _, c := range casos {
		if !isBareConfirmation(c) {
			t.Errorf("isBareConfirmation(%q) = false, esperaba true (auxiliar de haber + confirmación)", c)
		}
	}
}

// TestIsBareConfirmation_AuxiliarSoloNoConfirma confirma que la
// auxiliar de "haber" NO alcanza por sí sola para disparar una
// confirmación — solo cuenta si acompaña a una palabra de
// confirmación real, si no un input real que arranque con "ha"/"he"
// (aunque sea poco común) podría fusionarse mal por el motivo
// contrario.
func TestIsBareConfirmation_AuxiliarSoloNoConfirma(t *testing.T) {
	casos := []string{"ha", "he", "han", "hemos"}
	for _, c := range casos {
		if isBareConfirmation(c) {
			t.Errorf("isBareConfirmation(%q) = true, esperaba false (auxiliar sola, sin palabra de confirmación)", c)
		}
	}
}

// TestWaitingForClarification_FusionPreservaVerbo (Integration Fix) es
// el caso end-to-end que reprodujo el bug real en producción: "aprende
// blender" -> MoveAsk -> "confirmar" NO debe convertirse en "Blender
// confirmar" (sin verbo, irreconocible corriente abajo). Con
// OriginalGoal crudo + isBareConfirmation, el objetivo fusionado debe
// quedar igual al original.
func TestWaitingForClarification_FusionPreservaVerbo(t *testing.T) {
	original := "aprende blender"
	respuesta := "confirmar"

	var fused string
	if isBareConfirmation(respuesta) {
		fused = original
	} else {
		fused = original + " " + respuesta
	}

	if fused != "aprende blender" {
		t.Errorf("objetivo fusionado = %q, esperaba %q (una confirmación pura no debe alterar el objetivo)", fused, original)
	}
}
