package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
)

type Identity struct {
	PID            int    `json:"pid"`
	ExecutableHash string `json:"executableHash"`
	Nonce          string `json:"nonce"`
	Address        string `json:"address"`
	StartedAt      string `json:"startedAt"`
}
type Health struct {
	PID            int    `json:"pid"`
	ExecutableHash string `json:"executableHash"`
	Protocol       int    `json:"protocol"`
	Proof          string `json:"proof"`
	Active, Queued int
}

func proof(i Identity, challenge string) string {
	h := hmac.New(sha256.New, []byte(i.Nonce))
	h.Write([]byte(challenge + "|" + i.ExecutableHash + "|" + strconv.Itoa(i.PID) + "|1"))
	return hex.EncodeToString(h.Sum(nil))
}
func VerifyHealth(i Identity, challenge string, h Health) bool {
	return len(i.Nonce) >= 16 && h.PID == i.PID && h.ExecutableHash == i.ExecutableHash && h.Protocol == 1 && hmac.Equal([]byte(h.Proof), []byte(proof(i, challenge)))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	c := r.URL.Query().Get("challenge")
	if r.Method != "GET" || len(c) < 32 || len(c) > 128 {
		http.Error(w, "invalid health challenge", 400)
		return
	}
	cap := s.Limiter.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Health{PID: s.Identity.PID, ExecutableHash: s.Identity.ExecutableHash, Protocol: 1, Proof: proof(s.Identity, c), Active: cap.Active, Queued: cap.Queued})
}
