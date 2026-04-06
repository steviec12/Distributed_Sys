package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type Record struct {
	Value   string `json:"value"`
	Version uint64 `json:"version"`
}

type SetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ReplicateRequest struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version uint64 `json:"version"`
}

var (
	storeMu sync.RWMutex
	store   = make(map[string]Record)
	version uint64
	nodeID  = "node-1"
	role    = "standalone"
	mode    = "single"
	leaderAddr string
	followerAddrs []string
	peerAddrs []string
	n = 1
	w = 1
	r = 1
	readRotation uint64
)

func main() {
	nodeID = os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = "node-1"
	}
	role = os.Getenv("ROLE")
	if role == "" {
		role = "standalone"
	}
	mode = os.Getenv("MODE")
	if mode == "" {
		mode = "single"
	}
	leaderAddr = os.Getenv("LEADER_ADDR")
	followerAddrs = parseAddresses(os.Getenv("FOLLOWER_ADDRS"))
	peerAddrs = parseAddresses(os.Getenv("PEER_ADDRS"))
	n = parseIntEnv("N", 1)
	w = parseIntEnv("W", 1)
	r = parseIntEnv("R", 1)

	router := setupRouter()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := router.Run(":" + port); err != nil {
		panic(err)
	}
}

func setupRouter() *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":      "ok",
			"node_id":     nodeID,
			"role":        role,
			"mode":        mode,
			"leader_addr": leaderAddr,
			"peer_count":  len(peerAddrs),
			"n":           n,
			"w":           w,
			"r":           r,
		})
	})
	router.POST("/set", setHandler)
	router.GET("/get", getHandler)
	router.GET("/local_read", localReadHandler)
	router.POST("/internal/replicate", replicateHandler)
	router.GET("/internal/read", internalReadHandler)

	return router
}

func setHandler(c *gin.Context) {
	var req SetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	if req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "key cannot be empty",
		})
		return
	}

	if mode == "leader_follower" && role != "leader" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "writes must be sent to the leader",
		})
		return
	}

	storeMu.Lock()
	version++
	record := Record{
		Value:   req.Value,
		Version: version,
	}
	store[req.Key] = record
	storeMu.Unlock()

	if mode == "leaderless" {
		if err := replicateToPeers(req.Key, req.Value, record.Version); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	if mode == "leader_follower" && role == "leader" {
		if err := replicateToFollowers(req.Key, req.Value, record.Version); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"key":     req.Key,
		"value":   record.Value,
		"version": record.Version,
	})
}

func getHandler(c *gin.Context) {
	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "key cannot be empty",
		})
		return
	}

	if mode == "leader_follower" {
		if r == 1 {
			respondWithLocalRecord(c, key)
			return
		}

		if role != "leader" {
			proxyReadToLeader(c, key)
			return
		}

		record, ok, replicasUsed, err := coordinatedRead(key, r)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"error": err.Error(),
			})
			return
		}
		if len(replicasUsed) > 0 {
			c.Header("X-Read-Replicas", strings.Join(replicasUsed, ","))
		}

		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "key not found",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"key":     key,
			"value":   record.Value,
			"version": record.Version,
		})
		return
	}

	respondWithLocalRecord(c, key)
}

func localReadHandler(c *gin.Context) {
	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "key cannot be empty",
		})
		return
	}

	respondWithLocalRecord(c, key)
}

func replicateHandler(c *gin.Context) {
	var req ReplicateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	if req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "key cannot be empty",
		})
		return
	}

	storeMu.Lock()
	current, ok := store[req.Key]
	if !ok || req.Version >= current.Version {
		store[req.Key] = Record{
			Value:   req.Value,
			Version: req.Version,
		}
	}
	if req.Version > version {
		version = req.Version
	}
	storeMu.Unlock()

	time.Sleep(100 * time.Millisecond)

	c.JSON(http.StatusCreated, gin.H{
		"key":     req.Key,
		"value":   req.Value,
		"version": req.Version,
	})
}

func internalReadHandler(c *gin.Context) {
	key := c.Query("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "key cannot be empty",
		})
		return
	}

	if mode == "leader_follower" && role == "follower" {
		time.Sleep(50 * time.Millisecond)
	}

	storeMu.RLock()
	record, ok := store[key]
	storeMu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "key not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":     key,
		"value":   record.Value,
		"version": record.Version,
	})
}

func respondWithLocalRecord(c *gin.Context, key string) {
	record, ok := readLocalRecord(key)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "key not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"key":     key,
		"value":   record.Value,
		"version": record.Version,
	})
}

func readLocalRecord(key string) (Record, bool) {
	storeMu.RLock()
	record, ok := store[key]
	storeMu.RUnlock()
	return record, ok
}

func proxyReadToLeader(c *gin.Context, key string) {
	if leaderAddr == "" {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": "leader address is not configured",
		})
		return
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(leaderAddr + "/get?key=" + url.QueryEscape(key))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("proxy read to leader failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("failed to read leader response: %v", err),
		})
		return
	}

	if replicasUsed := resp.Header.Get("X-Read-Replicas"); replicasUsed != "" {
		c.Header("X-Read-Replicas", replicasUsed)
	}

	c.Data(resp.StatusCode, "application/json; charset=utf-8", body)
}

func coordinatedRead(key string, readCount int) (Record, bool, []string, error) {
	if readCount < 1 {
		return Record{}, false, nil, fmt.Errorf("r must be at least 1")
	}

	if readCount > n {
		return Record{}, false, nil, fmt.Errorf("r=%d exceeds n=%d", readCount, n)
	}

	record, found := readLocalRecord(key)
	neededFollowerReads := readCount - 1
	if neededFollowerReads > len(followerAddrs) {
		return Record{}, false, nil, fmt.Errorf("r=%d requires %d follower reads, but only %d followers are configured", readCount, neededFollowerReads, len(followerAddrs))
	}

	replicasUsed := []string{nodeID}
	startIndex := 0
	if len(followerAddrs) > 0 {
		startIndex = int((atomic.AddUint64(&readRotation, 1) - 1) % uint64(len(followerAddrs)))
	}

	for i := 0; i < neededFollowerReads; i++ {
		followerIndex := (startIndex + i) % len(followerAddrs)
		followerRecord, followerFound, err := readFromFollower(followerAddrs[followerIndex], key)
		if err != nil {
			return Record{}, false, nil, err
		}
		replicasUsed = append(replicasUsed, fmt.Sprintf("node-%d", followerIndex+2))

		if followerFound && (!found || followerRecord.Version > record.Version) {
			record = followerRecord
			found = true
		}
	}

	return record, found, replicasUsed, nil
}

func readFromFollower(addr, key string) (Record, bool, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(addr + "/internal/read?key=" + url.QueryEscape(key))
	if err != nil {
		return Record{}, false, fmt.Errorf("read from %s failed: %w", addr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Record{}, false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return Record{}, false, fmt.Errorf("read from %s returned status %d", addr, resp.StatusCode)
	}

	var body struct {
		Key     string `json:"key"`
		Value   string `json:"value"`
		Version uint64 `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Record{}, false, fmt.Errorf("decode read from %s: %w", addr, err)
	}

	return Record{
		Value:   body.Value,
		Version: body.Version,
	}, true, nil
}

func replicateToFollowers(key, value string, version uint64) error {
	requiredFollowerAcks := w - 1
	if requiredFollowerAcks <= 0 {
		go replicateToRemainingFollowers(key, value, version, append([]string(nil), followerAddrs...))
		return nil
	}

	if requiredFollowerAcks > len(followerAddrs) {
		return fmt.Errorf("w=%d requires %d follower acknowledgments, but only %d followers are configured", w, requiredFollowerAcks, len(followerAddrs))
	}

	ackedFollowers := 0
	for i, addr := range followerAddrs {
		if err := replicateToFollower(addr, key, value, version); err != nil {
			return err
		}

		ackedFollowers++
		if ackedFollowers >= requiredFollowerAcks {
			remaining := append([]string(nil), followerAddrs[i+1:]...)
			if len(remaining) > 0 {
				go replicateToRemainingFollowers(key, value, version, remaining)
			}
			return nil
		}
	}

	return nil
}

func replicateToPeers(key, value string, version uint64) error {
	requiredPeerAcks := w - 1
	if requiredPeerAcks != len(peerAddrs) {
		return fmt.Errorf(
			"leaderless mode requires acknowledgments from all peers: need %d, configured %d",
			requiredPeerAcks,
			len(peerAddrs),
		)
	}

	for _, addr := range peerAddrs {
		if err := replicateToFollower(addr, key, value, version); err != nil {
			return err
		}
	}

	return nil
}

func replicateToFollower(addr, key, value string, version uint64) error {
	body, err := json.Marshal(ReplicateRequest{
		Key:     key,
		Value:   value,
		Version: version,
	})
	if err != nil {
		return fmt.Errorf("marshal replicate request: %w", err)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(addr+"/internal/replicate", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("replicate to %s: %w", addr, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("replicate to %s returned status %d", addr, resp.StatusCode)
	}

	time.Sleep(200 * time.Millisecond)

	return nil
}

func replicateToRemainingFollowers(key, value string, version uint64, addrs []string) {
	for _, addr := range addrs {
		if err := replicateToFollower(addr, key, value, version); err != nil {
			fmt.Fprintf(os.Stderr, "background replication failed for %s: %v\n", addr, err)
		}
	}
}

func parseAddresses(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	addresses := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			addresses = append(addresses, part)
		}
	}

	return addresses
}

func parseIntEnv(name string, defaultValue int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return defaultValue
	}

	return value
}
