package commons

import (
	"bufio"
	"github.com/xpanvictor/raft/configs"
	"os"
)

func ArrStrFromFile(file *os.File) []string {
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func HasPriorityVotes(totalClients int, totalVotes int) bool {
	majority := float32(totalClients) * configs.PRIORITY_VOTES
	if float32(totalVotes) >= majority {
		return true
	}
	return false
}
