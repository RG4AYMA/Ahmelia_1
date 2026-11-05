package environment

import "strings"

// ============================================================================
// FRIENDLY.GO — traducción de nombre de proceso a nombre natural
// ============================================================================
//
// scan.go ya resuelve QUÉ hay abierto (vía Get-Process), pero devuelve
// nombres de proceso crudos: "steamwebhelper", "taskmgr", "code". Eso es
// correcto para el motor pero incómodo para hablar. Este archivo es la
// única capa que traduce "proceso" -> "cómo lo diría una persona", para
// que Describe() (y cualquier respuesta hablada) no repita nombres crudos.
//
// No se mete con WorldState ni con AppState: es puro lookup, sin estado,
// así que Manager solo lo llama al momento de construir texto.

// AppCategory agrupa procesos por tipo, pensado para responder preguntas
// como "¿qué navegadores tengo abiertos?" sin depender del nombre exacto
// del proceso.
type AppCategory string

const (
	CategoryBrowser  AppCategory = "navegador"
	CategoryDev      AppCategory = "desarrollo"
	CategoryGame     AppCategory = "juego"
	CategoryLauncher AppCategory = "launcher" // Steam, Epic, etc: abren juegos, no SON un juego
	CategoryMedia    AppCategory = "media"
	CategoryOffice   AppCategory = "ofimática"
	CategorySystem   AppCategory = "sistema"
	CategoryOther    AppCategory = "otro"
)

// appInfo es lo que sabemos de un proceso conocido: cómo llamarlo, en qué
// categoría cae, y si es "secundario" dentro de su categoría.
//
// Secondary existe por un caso real: cmd/powershell caen en Desarrollo
// (son herramientas de dev), pero una consola abierta no significa que
// alguien esté desarrollando — es una señal mucho más débil que tener
// VSCode o Unity abiertos. Secondary=true marca esas herramientas de
// apoyo para que DescribeByCategory() en manager.go pueda decir "solo
// veo abierta una consola" en vez de afirmar de más.
type appInfo struct {
	Name      string
	Category  AppCategory
	Secondary bool
}

// knownApps es el diccionario de procesos reconocidos. Las llaves están
// normalizadas: minúsculas, sin ".exe". Se amplía con datos reales según
// aparezcan (igual que se ajustó el filtro de scan.go con la salida real
// del usuario), no adivinando por adelantado.
var knownApps = map[string]appInfo{
	// Navegadores
	"chrome":  {Name: "Chrome", Category: CategoryBrowser},
	"msedge":  {Name: "Edge", Category: CategoryBrowser},
	"firefox": {Name: "Firefox", Category: CategoryBrowser},
	"opera":   {Name: "Opera", Category: CategoryBrowser},
	"brave":   {Name: "Brave", Category: CategoryBrowser},

	// Desarrollo — nivel principal: IDEs y motores. Que estén abiertos
	// SÍ implica que alguien está desarrollando.
	"code":      {Name: "Visual Studio Code", Category: CategoryDev},
	"devenv":    {Name: "Visual Studio", Category: CategoryDev},
	"unity":     {Name: "Unity", Category: CategoryDev},
	"unityhub":  {Name: "Unity Hub", Category: CategoryDev},
	"blender":   {Name: "Blender", Category: CategoryDev},
	"pycharm64": {Name: "PyCharm", Category: CategoryDev},
	"idea64":    {Name: "IntelliJ IDEA", Category: CategoryDev},

	// Desarrollo — nivel secundario: terminales. Son herramienta de
	// apoyo, no prueban por sí solas que haya desarrollo en curso.
	"cmd":             {Name: "la consola", Category: CategoryDev, Secondary: true},
	"windowsterminal": {Name: "la terminal", Category: CategoryDev, Secondary: true},
	"powershell":      {Name: "PowerShell", Category: CategoryDev, Secondary: true},

	// Launchers — abren juegos, pero abiertos por sí solos no significa
	// que haya un juego corriendo (ver doc de feedback: "Steam abierto
	// no significa que estés jugando").
	"steam":              {Name: "Steam", Category: CategoryLauncher},
	"steamwebhelper":     {Name: "Steam", Category: CategoryLauncher},
	"epicgameslauncher":  {Name: "Epic Games", Category: CategoryLauncher},
	"galaxyclient":       {Name: "GOG Galaxy", Category: CategoryLauncher},
	"battlenet":          {Name: "Battle.net", Category: CategoryLauncher},
	"origin":             {Name: "Origin", Category: CategoryLauncher},
	"eaapp":              {Name: "EA App", Category: CategoryLauncher},
	"ubisoftconnect":     {Name: "Ubisoft Connect", Category: CategoryLauncher},

	// Media
	"vlc":     {Name: "VLC", Category: CategoryMedia},
	"spotify": {Name: "Spotify", Category: CategoryMedia},
	"discord": {Name: "Discord", Category: CategoryMedia},
	"obs64":   {Name: "OBS Studio", Category: CategoryMedia},

	// Ofimática / utilidades
	"winrar":   {Name: "WinRAR", Category: CategoryOffice},
	"7zfm":     {Name: "7-Zip", Category: CategoryOffice},
	"notepad":  {Name: "el Bloc de notas", Category: CategoryOffice},
	"winword":  {Name: "Word", Category: CategoryOffice},
	"excel":    {Name: "Excel", Category: CategoryOffice},
	"powerpnt": {Name: "PowerPoint", Category: CategoryOffice},
	"acrobat":  {Name: "Acrobat", Category: CategoryOffice},
	"acrord32": {Name: "Adobe Reader", Category: CategoryOffice},

	// Sistema (con ventana visible, pero igual valen nombre amigable)
	"taskmgr":         {Name: "el Administrador de tareas", Category: CategorySystem},
	"explorer":        {Name: "el Explorador de archivos", Category: CategorySystem},
	"nvidia overlay":  {Name: "la superposición de NVIDIA", Category: CategorySystem},
	"nvcontainer":     {Name: "el servicio de NVIDIA", Category: CategorySystem},
}

// lookup normaliza un nombre de proceso y busca su appInfo. Punto único
// que usan FriendlyName, CategoryOf e isSecondaryDev, para no repetir
// la normalización tres veces.
func lookup(process string) (appInfo, bool) {
	key := strings.ToLower(strings.TrimSpace(process))
	key = strings.TrimSuffix(key, ".exe")
	info, ok := knownApps[key]
	return info, ok
}

// FriendlyName devuelve cómo debería decirse un proceso en voz alta.
// Si no está en el diccionario, aplica un plan B razonable: sin ".exe",
// espacios en vez de guiones/underscores, y con mayúscula inicial —
// mejor que un nombre de proceso crudo, aunque no sea perfecto.
func FriendlyName(process string) string {
	if info, ok := lookup(process); ok {
		return info.Name
	}

	key := strings.ToLower(strings.TrimSpace(process))
	key = strings.TrimSuffix(key, ".exe")
	fallback := strings.ReplaceAll(key, "_", " ")
	fallback = strings.ReplaceAll(fallback, "-", " ")
	if fallback == "" {
		return process
	}
	return strings.ToUpper(fallback[:1]) + fallback[1:]
}

// CategoryOf devuelve la categoría de un proceso conocido. Para procesos
// fuera del diccionario devuelve CategoryOther — no inventamos categoría,
// igual que no se inventaron sensores que no existen en state.go.
func CategoryOf(process string) AppCategory {
	if info, ok := lookup(process); ok {
		return info.Category
	}
	return CategoryOther
}

// isSecondaryDev distingue, DENTRO de CategoryDev, una terminal (señal
// débil de "estoy desarrollando") de un IDE/motor (señal fuerte).
func isSecondaryDev(process string) bool {
	info, ok := lookup(process)
	return ok && info.Secondary
}

// ============================================================================
// Consulta por categoría ("¿qué navegadores tienes abiertos?")
// ============================================================================
//
// categoryKeywords son las palabras que, si aparecen en el texto YA
// normalizado (minúsculas, sin tildes — mismo normalizador que usa el
// resto del engine), identifican de qué categoría está preguntando el
// usuario. No intenta ser un NLU completo: es el mismo enfoque simple de
// coincidencia de texto que ya usa engine.go para "qué tienes abierto".
var categoryKeywords = map[AppCategory][]string{
	CategoryBrowser:  {"navegador", "navegadores"},
	CategoryDev:      {"desarrollo", "programacion", "codigo", "editor"},
	CategoryGame:     {"juego", "juegos"},
	CategoryLauncher: {"launcher", "lanzador"},
	CategoryMedia:    {"musica", "reproductor", "multimedia", "video"},
	CategoryOffice:   {"oficina", "ofimatica", "documento", "documentos"},
	CategorySystem:   {"sistema"},
}

// ParseCategoryQuery revisa si el texto (ya normalizado) pregunta por una
// categoría específica de aplicaciones. Se espera que el texto ya haya
// pasado por shared.Normalize antes de llegar aquí — este archivo no
// depende de ese paquete para no acoplar environment a normalización de
// texto que no le pertenece.
func ParseCategoryQuery(normalizedText string) (AppCategory, bool) {
	for cat, keywords := range categoryKeywords {
		for _, kw := range keywords {
			if strings.Contains(normalizedText, kw) {
				return cat, true
			}
		}
	}
	return "", false
}

// categoryLabel es cómo se nombra la categoría dentro de una respuesta
// hablada, ej. "Tienes 2 navegadores abiertos".
func categoryLabel(cat AppCategory, plural bool) string {
	labels := map[AppCategory][2]string{
		CategoryBrowser:  {"navegador", "navegadores"},
		CategoryDev:      {"programa de desarrollo", "programas de desarrollo"},
		CategoryGame:     {"juego", "juegos"},
		CategoryLauncher: {"launcher de juegos", "launchers de juegos"},
		CategoryMedia:    {"programa de media", "programas de media"},
		CategoryOffice:   {"programa de ofimática", "programas de ofimática"},
		CategorySystem:   {"programa de sistema", "programas de sistema"},
		CategoryOther:    {"programa", "programas"},
	}
	pair, ok := labels[cat]
	if !ok {
		pair = labels[CategoryOther]
	}
	if plural {
		return pair[1]
	}
	return pair[0]
}
