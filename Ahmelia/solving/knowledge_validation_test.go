package solving

import (
	"ahmelia/interaction"
	"strings"
	"testing"
	"time"

	"ahmelia/gateway"
)

// TestValidate_DocumentoConfiable confirma la ruta central: StatusCode
// 200 y contenido suficiente producen IsReliable()==true, sin Issues.
func TestValidate_DocumentoConfiable(t *testing.T) {
	doc := &gateway.Document{
		URL: "https://docs.unity3d.com/", Source: "Unity Docs",
		Text:       strings.Repeat("contenido real de la documentación. ", 20),
		StatusCode: 200, FetchedAt: time.Now(),
	}

	sv := NewSourceValidator()
	v := sv.Validate(doc)

	if !v.IsReliable() {
		t.Fatalf("esperaba IsReliable()==true, obtuve Issues=%v", v.Issues)
	}
	if len(v.Issues) != 0 {
		t.Errorf("un documento confiable no debería tener Issues, obtuve %v", v.Issues)
	}
}

// TestValidate_StatusNo2xxNoEsConfiable confirma que un StatusCode
// fuera del rango 2xx queda IsReliable()==false, con el Issue
// correspondiente.
func TestValidate_StatusNo2xxNoEsConfiable(t *testing.T) {
	doc := &gateway.Document{
		URL: "https://docs.unity3d.com/perdido", StatusCode: 404,
		Text: strings.Repeat("x", 500),
	}

	sv := NewSourceValidator()
	v := sv.Validate(doc)

	if v.IsReliable() {
		t.Fatal("un 404 no debería ser IsReliable()")
	}
	if v.StatusOK {
		t.Error("StatusOK debería ser false para un 404")
	}
	if len(v.Issues) != 1 || !strings.Contains(v.Issues[0], "404") {
		t.Errorf("esperaba un Issue mencionando el 404, obtuve %v", v.Issues)
	}
}

// TestValidate_PocoContenidoNoEsConfiable confirma que un documento con
// texto por debajo de MinContentLength queda IsReliable()==false, aun
// con StatusCode 200.
func TestValidate_PocoContenidoNoEsConfiable(t *testing.T) {
	doc := &gateway.Document{URL: "https://docs.unity3d.com/", StatusCode: 200, Text: "muy poco"}

	sv := NewSourceValidator()
	v := sv.Validate(doc)

	if v.IsReliable() {
		t.Fatal("un documento casi vacío no debería ser IsReliable()")
	}
	if !v.StatusOK {
		t.Error("StatusOK debería seguir siendo true — el problema es el contenido, no el status")
	}
}

// TestValidate_DocumentoNilDevuelveNil confirma el criterio defensivo
// — mismo bug que se encontró y corrigió en la sesión anterior: chequear
// doc==nil ANTES de tocar cualquier campo de doc, no después.
func TestValidate_DocumentoNilDevuelveNil(t *testing.T) {
	sv := NewSourceValidator()
	if sv.Validate(nil) != nil {
		t.Error("Validate(nil) debería devolver nil, no un *SourceValidation con campos vacíos")
	}
}

// TestValidateAll_CorroboradoConDosFuentesDistintas confirma el
// criterio central de corroboración: dos documentos confiables de DOS
// fuentes (Source) distintas marcan Corroborated=true.
func TestValidateAll_CorroboradoConDosFuentesDistintas(t *testing.T) {
	docs := []*gateway.Document{
		{URL: "https://docs.unity3d.com/", Source: "Unity Docs", StatusCode: 200, Text: strings.Repeat("x", 500)},
		{URL: "https://learn.microsoft.com/", Source: "Microsoft Learn", StatusCode: 200, Text: strings.Repeat("y", 500)},
	}

	sv := NewSourceValidator()
	vs := sv.ValidateAll("aprender c#", docs)

	if !vs.Corroborated {
		t.Fatal("dos fuentes distintas y confiables deberían marcar Corroborated=true")
	}
}

// TestValidateAll_MismaFuenteNoCorrobora confirma que dos documentos
// CONFIABLES pero de la MISMA fuente (Source) no cuentan como
// corroboración — es la misma fuente diciendo lo mismo dos veces, no
// una segunda opinión independiente.
func TestValidateAll_MismaFuenteNoCorrobora(t *testing.T) {
	docs := []*gateway.Document{
		{URL: "https://docs.unity3d.com/a", Source: "Unity Docs", StatusCode: 200, Text: strings.Repeat("x", 500)},
		{URL: "https://docs.unity3d.com/b", Source: "Unity Docs", StatusCode: 200, Text: strings.Repeat("y", 500)},
	}

	sv := NewSourceValidator()
	vs := sv.ValidateAll("aprender unity", docs)

	if vs.Corroborated {
		t.Fatal("dos documentos de la MISMA fuente no deberían corroborarse entre sí")
	}
}

// TestValidateAll_UnaSolaFuenteConfiableNoCorrobora confirma el caso
// mínimo: una sola fuente confiable, sin nada con qué compararla.
func TestValidateAll_UnaSolaFuenteConfiableNoCorrobora(t *testing.T) {
	docs := []*gateway.Document{
		{URL: "https://docs.unity3d.com/", Source: "Unity Docs", StatusCode: 200, Text: strings.Repeat("x", 500)},
	}

	sv := NewSourceValidator()
	vs := sv.ValidateAll("aprender unity", docs)

	if vs.Corroborated {
		t.Fatal("una sola fuente no puede corroborarse a sí misma")
	}
}

// TestDescribe_VacioEsHonesto confirma que un ValidationSet sin
// documentos lo dice explícito.
func TestValidationSet_Describe_VacioEsHonesto(t *testing.T) {
	vs := &ValidationSet{Goal: "aprender unreal"}
	desc := vs.Describe()

	if !strings.Contains(desc, "No hay ningún documento") {
		t.Errorf("esperaba el mensaje honesto de conjunto vacío, obtuve: %q", desc)
	}
}

// TestParseSourceValidationQuery_DistintoDeLearningValidator confirma
// que el trigger no se pisa con ParseValidatorQuery (Bloque 48,
// fiabilidad de ActionConfidence — un concepto interno completamente
// distinto de fiabilidad de documentos externos).
func TestParseSourceValidationQuery_DistintoDeLearningValidator(t *testing.T) {
	if !ParseSourceValidationQuery(interaction.Analyze("esa fuente es confiable", interaction.Context{})) {
		t.Error("esperaba detectar el trigger de Knowledge Validation")
	}
	if ParseSourceValidationQuery(interaction.Analyze("que tan confiable es eso que aprendiste", interaction.Context{})) {
		t.Error("un pedido de Learning Validator (Bloque 48) no debería disparar Knowledge Validation")
	}
}
