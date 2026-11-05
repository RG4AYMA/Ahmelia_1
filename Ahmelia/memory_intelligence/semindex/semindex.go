// Package semindex es el índice de recuerdos de Ahmelia: recuperación híbrida
// (densa + léxica BM25) fusionada por RRF, con decaimiento temporal para
// recuerdos episódicos y journal durable con fsync.
//
// No decide intención ni acción: devuelve EVIDENCIA ordenada (R5/R6).
package semindex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Kind string

const (
	KindEpisodic Kind = "episodic" // qué pasó (con tiempo): decae
	KindSemantic Kind = "semantic" // qué es verdad (consolidado): no decae
	KindWorking  Kind = "working"  // contexto vivo de la tarea actual: decae rápido
)

// Embedder es la costura hacia un modelo local. ID identifica el espacio
// vectorial: vectores de distinto ID NUNCA se comparan.
type Embedder interface {
	ID() string
	Embed(ctx context.Context, text string) ([]float32, error)
}

// HashEmbedder: embedder determinista sin red (feature hashing de palabras y
// trigramas de caracteres, plegado de acentos). Es el piso de operación sin
// Ollama; NO es comprensión semántica profunda (sinónimos no se unen).
type HashEmbedder struct{ D int }

func (h HashEmbedder) ID() string { return "hash-v1" }

func (h HashEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	d := h.D
	if d <= 0 {
		d = 512
	}
	v := make([]float32, d)
	add := func(f string, w float32) {
		x := fnv.New64a()
		_, _ = x.Write([]byte(f))
		s := x.Sum64()
		sign := float32(1)
		if s&(1<<63) != 0 {
			sign = -1
		}
		v[int(s%uint64(d))] += sign * w
	}
	for _, t := range Tokens(text) {
		add("w:"+t, 1)
		r := []rune("^" + t + "$")
		for i := 0; i+3 <= len(r); i++ {
			add("c:"+string(r[i:i+3]), 0.5)
		}
	}
	return normalize(v), nil
}

func normalize(v []float32) []float32 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	if n == 0 {
		return v
	}
	n = math.Sqrt(n)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return v
}

var fold = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// Tokens: minúsculas, sin acentos, palabras >= 2 runas.
func Tokens(s string) []string {
	s = fold.Replace(strings.ToLower(s))
	f := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' })
	out := f[:0]
	for _, w := range f {
		if len([]rune(w)) >= 2 {
			out = append(out, w)
		}
	}
	return out
}

type Entry struct {
	ID      string            `json:"id"`
	Kind    Kind              `json:"kind"`
	Text    string            `json:"text"`
	Meta    map[string]string `json:"meta,omitempty"` // work_id, thread_id, capability...
	Refs    []string          `json:"refs,omitempty"` // evidencia trazable
	At      time.Time         `json:"at"`
	EmbID   string            `json:"emb_id"`
	Vec     []float32         `json:"vec,omitempty"`
	Deleted bool              `json:"deleted,omitempty"`
	tf      map[string]int
	dl      int
}

type Index struct {
	mu   sync.RWMutex
	emb  Embedder
	path string
	byID map[string]*Entry
}

// Open reproduce el journal. Una línea corrupta se omite pero se cuenta.
func Open(path string, emb Embedder) (*Index, int, error) {
	ix := &Index{emb: emb, path: path, byID: map[string]*Entry{}}
	if path == "" {
		return ix, 0, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return ix, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	bad := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.ID == "" {
			bad++
			continue
		}
		if e.Deleted {
			delete(ix.byID, e.ID)
			continue
		}
		prep(&e)
		ix.byID[e.ID] = &e
	}
	return ix, bad, sc.Err()
}

func prep(e *Entry) {
	e.tf = map[string]int{}
	toks := Tokens(e.Text)
	for _, t := range toks {
		e.tf[t]++
	}
	e.dl = len(toks)
}

func (ix *Index) journal(e *Entry) error {
	if ix.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ix.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(ix.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(e)
	if _, err = f.Write(append(b, '\n')); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Add es idempotente por ID (upsert). Si el embedder falla, el recuerdo se
// guarda igual (búsqueda léxica) y se re-embebe luego con Reembed.
func (ix *Index) Add(ctx context.Context, e Entry) error {
	if e.ID == "" || strings.TrimSpace(e.Text) == "" {
		return errors.New("semindex: id and text required")
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if ix.emb != nil {
		if v, err := ix.emb.Embed(ctx, e.Text); err == nil {
			e.Vec, e.EmbID = v, ix.emb.ID()
		}
	}
	prep(&e)
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.byID[e.ID] = &e
	return ix.journal(&e)
}

func (ix *Index) Delete(id string) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	delete(ix.byID, id)
	return ix.journal(&Entry{ID: id, Deleted: true})
}

// Reembed recalcula vectores cuyo espacio no coincide con el embedder actual.
func (ix *Index) Reembed(ctx context.Context) (int, error) {
	if ix.emb == nil {
		return 0, nil
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	n := 0
	for _, e := range ix.byID {
		if e.EmbID == ix.emb.ID() && len(e.Vec) > 0 {
			continue
		}
		v, err := ix.emb.Embed(ctx, e.Text)
		if err != nil {
			return n, err
		}
		e.Vec, e.EmbID = v, ix.emb.ID()
		if err := ix.journal(e); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

type Options struct {
	K         int
	Kinds     []Kind
	Now       time.Time
	HalfLife  map[Kind]time.Duration // por defecto: episodic 14d, working 2h
	MetaMatch map[string]string
}

type Hit struct {
	Entry     Entry
	Score     float64
	DenseRank int // 0 = no participó
	LexRank   int
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// Search: BM25 + coseno -> RRF(k=60) -> decaimiento -> top K.
func (ix *Index) Search(ctx context.Context, q string, o Options) ([]Hit, error) {
	if o.K <= 0 {
		o.K = 5
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	hl := map[Kind]time.Duration{KindEpisodic: 14 * 24 * time.Hour, KindWorking: 2 * time.Hour}
	for k, v := range o.HalfLife {
		hl[k] = v
	}
	var qv []float32
	if ix.emb != nil {
		if v, err := ix.emb.Embed(ctx, q); err == nil {
			qv = v
		}
	}
	qt := Tokens(q)

	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var cand []*Entry
	for _, e := range ix.byID {
		if len(o.Kinds) > 0 {
			ok := false
			for _, k := range o.Kinds {
				ok = ok || k == e.Kind
			}
			if !ok {
				continue
			}
		}
		match := true
		for k, v := range o.MetaMatch {
			match = match && e.Meta[k] == v
		}
		if match {
			cand = append(cand, e)
		}
	}
	N := float64(len(cand))
	df := map[string]int{}
	var avg float64
	for _, e := range cand {
		avg += float64(e.dl)
		for _, t := range qt {
			if e.tf[t] > 0 {
				df[t]++
			}
		}
	}
	if N > 0 {
		avg /= N
	}
	type sc struct {
		e *Entry
		s float64
	}
	var lex, den []sc
	for _, e := range cand {
		var s float64
		for _, t := range qt {
			f := float64(e.tf[t])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (N-float64(df[t])+0.5)/(float64(df[t])+0.5))
			s += idf * f * 2.2 / (f + 1.2*(0.25+0.75*float64(e.dl)/math.Max(avg, 1)))
		}
		if s > 0 {
			lex = append(lex, sc{e, s})
		}
		if qv != nil && e.EmbID == ix.emb.ID() {
			if c := cosine(qv, e.Vec); c > 0.05 {
				den = append(den, sc{e, c})
			}
		}
	}
	rank := func(x []sc) map[string]int {
		sort.SliceStable(x, func(i, j int) bool { return x[i].s > x[j].s })
		m := map[string]int{}
		for i, v := range x {
			m[v.e.ID] = i + 1
		}
		return m
	}
	lr, dr := rank(lex), rank(den)
	var hits []Hit
	seen := map[string]bool{}
	for _, list := range [][]sc{lex, den} {
		for _, v := range list {
			if seen[v.e.ID] {
				continue
			}
			seen[v.e.ID] = true
			s := 0.0
			if r := lr[v.e.ID]; r > 0 {
				s += 1 / float64(60+r)
			}
			if r := dr[v.e.ID]; r > 0 {
				s += 1 / float64(60+r)
			}
			if h, ok := hl[v.e.Kind]; ok && h > 0 {
				s *= math.Pow(0.5, math.Max(0, o.Now.Sub(v.e.At).Seconds())/h.Seconds())
			}
			cp := *v.e
			cp.Vec = nil
			hits = append(hits, Hit{Entry: cp, Score: s, DenseRank: dr[v.e.ID], LexRank: lr[v.e.ID]})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > o.K {
		hits = hits[:o.K]
	}
	return hits, nil
}

// Compact reescribe el journal de forma atómica (tmp + fsync + rename).
func (ix *Index) Compact() error {
	if ix.path == "" {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	tmp := ix.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(ix.byID))
	for id := range ix.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	w := bufio.NewWriter(f)
	for _, id := range ids {
		b, _ := json.Marshal(ix.byID[id])
		_, _ = w.Write(append(b, '\n'))
	}
	if err = w.Flush(); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, ix.path)
}
