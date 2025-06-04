package ratelimit

import (
	"log"
	"sync"
)

var (
	ipConnectionCounts = make(map[string]int)
	ipCountsMutex      = &sync.Mutex{}
)

const rateLimitLogPrefix = "RATE_LIMIT"

// AllowConnection checks if a new connection from the given IP is allowed based on maxConnections.
// If allowed, it increments the count for that IP and returns true. Otherwise, returns false.
func AllowConnection(ip string, maxConnections int) bool {
	ipCountsMutex.Lock()
	defer ipCountsMutex.Unlock()

	currentCount := ipConnectionCounts[ip] // Defaults to 0 if IP not in map

	if maxConnections > 0 && currentCount >= maxConnections {
		log.Printf("INFO: %s: Connection attempt from %s denied. Current connections: %d, Limit: %d",
			rateLimitLogPrefix, ip, currentCount, maxConnections)
		return false
	}

	ipConnectionCounts[ip] = currentCount + 1
	log.Printf("DEBUG: %s: Connection from %s allowed. New count: %d (Limit: %d)",
		rateLimitLogPrefix, ip, ipConnectionCounts[ip], maxConnections)
	return true
}

// DecrementConnectionCount decrements the connection count for the given IP.
// If the count drops to 0, the IP is removed from the map to prevent bloat.
func DecrementConnectionCount(ip string) {
	ipCountsMutex.Lock()
	defer ipCountsMutex.Unlock()

	currentCount, ok := ipConnectionCounts[ip]
	if !ok {
		log.Printf("WARN: %s: Attempted to decrement connection count for IP %s not in map.", rateLimitLogPrefix, ip)
		return
	}

	if currentCount <= 1 {
		delete(ipConnectionCounts, ip)
		log.Printf("DEBUG: %s: Connection from %s closed. Count was %d, IP removed from tracking.", rateLimitLogPrefix, ip, currentCount)
	} else {
		ipConnectionCounts[ip] = currentCount - 1
		log.Printf("DEBUG: %s: Connection from %s closed. New count: %d.", rateLimitLogPrefix, ip, ipConnectionCounts[ip])
	}
}
