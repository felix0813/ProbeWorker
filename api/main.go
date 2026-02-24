package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Probe struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	User      string    `json:"user,omitempty"`
	Password  string    `json:"password,omitempty"`
	DBName    string    `json:"dbname,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ProbeHistory struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CheckedAt time.Time `json:"checkedAt"`
}

type Store struct {
	mu      sync.RWMutex
	probes  map[string]*Probe
	history map[string][]ProbeHistory
	file    string
}

func NewStore(file string) (*Store, error) {
	s := &Store{probes: map[string]*Probe{}, history: map[string][]ProbeHistory{}, file: file}
	return s, s.load()
}
func (s *Store) load() error {
	if _, err := os.Stat(s.file); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	b, err := os.ReadFile(s.file)
	if err != nil {
		return err
	}
	var list []*Probe
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	for _, p := range list {
		cp := *p
		s.probes[p.ID] = &cp
	}
	return nil
}
func (s *Store) save() error {
	list := make([]*Probe, 0, len(s.probes))
	for _, p := range s.probes {
		cp := *p
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.file, b, 0o644)
}
func (s *Store) list() []*Probe {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*Probe, 0, len(s.probes))
	for _, p := range s.probes {
		cp := *p
		list = append(list, &cp)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
	return list
}
func (s *Store) upsert(p *Probe) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if old, ok := s.probes[p.ID]; ok {
		p.CreatedAt = old.CreatedAt
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	cp := *p
	s.probes[p.ID] = &cp
	return s.save()
}
func (s *Store) get(id string) (*Probe, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.probes[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}
func (s *Store) delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.probes, id)
	delete(s.history, id)
	return s.save()
}
func (s *Store) appendHistory(id string, h ProbeHistory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[id] = append([]ProbeHistory{h}, s.history[id]...)
	if len(s.history[id]) > 50 {
		s.history[id] = s.history[id][:50]
	}
}
func (s *Store) getHistory(id string) []ProbeHistory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := s.history[id]
	res := make([]ProbeHistory, len(items))
	copy(res, items)
	return res
}

func main() {
	dataFile := getenv("API_DATA_FILE", "data/probes.json")
	webDir := getenv("API_WEB_DIR", "../web")
	listenAddr := getenv("API_LISTEN_ADDR", ":8080")

	store, err := NewStore(dataFile)
	if err != nil {
		log.Fatalf("init store failed: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/probes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.list())
		case http.MethodPost:
			var p Probe
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			if err := validateProbe(&p); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if p.ID == "" {
				p.ID = fmt.Sprintf("probe-%d", time.Now().UnixNano())
			}
			if err := store.upsert(&p); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, p)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/probes/check-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		results := map[string]ProbeHistory{}
		for _, p := range store.list() {
			if !p.Enabled {
				continue
			}
			h := runCheck(*p)
			store.appendHistory(p.ID, h)
			results[p.ID] = h
		}
		writeJSON(w, http.StatusOK, results)
	})
	mux.HandleFunc("/api/probes/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/probes/"), "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		id := parts[0]
		if len(parts) == 1 {
			switch r.Method {
			case http.MethodPut:
				old, ok := store.get(id)
				if !ok {
					http.NotFound(w, r)
					return
				}
				var p Probe
				if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				p.ID = id
				if p.Password == "" {
					p.Password = old.Password
				}
				if err := validateProbe(&p); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := store.upsert(&p); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, http.StatusOK, p)
			case http.MethodDelete:
				if err := store.delete(id); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		switch parts[1] {
		case "check":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			p, ok := store.get(id)
			if !ok {
				http.NotFound(w, r)
				return
			}
			h := runCheck(*p)
			store.appendHistory(id, h)
			writeJSON(w, http.StatusOK, h)
		case "history":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			writeJSON(w, http.StatusOK, store.getHistory(id))
		default:
			http.NotFound(w, r)
		}
	})
	mux.Handle("/", http.FileServer(http.Dir(webDir)))
	log.Printf("server listening on %s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, withCORS(mux)))
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runCheck(p Probe) ProbeHistory {
	addr := fmt.Sprintf("%s:%d", p.Host, p.Port)
	conn, err := net.DialTimeout("tcp", addr, 4*time.Second)
	if err != nil {
		return ProbeHistory{Status: "abnormal", Message: err.Error(), CheckedAt: time.Now()}
	}
	_ = conn.Close()
	return ProbeHistory{Status: "normal", Message: "tcp connect success", CheckedAt: time.Now()}
}
func validateProbe(p *Probe) error {
	if p.Name == "" || p.Type == "" || p.Host == "" || p.Port <= 0 {
		return errors.New("name/type/host/port are required")
	}
	if p.Type != "postgres" && p.Type != "pg" && p.Type != "redis" {
		return errors.New("type must be postgres/pg/redis")
	}
	if (p.Type == "postgres" || p.Type == "pg") && (p.User == "" || p.DBName == "") {
		return errors.New("postgres probe requires user and dbname")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
