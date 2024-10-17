package configs

import "time"

const (
	ELECTION_TIMEOUT = 2 * time.Second
	PRIORITY_VOTES   = float32(1.0 / 3.0)
)
