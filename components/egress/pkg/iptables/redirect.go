package iptables

import (
	"fmt"
	"os/exec"
	"strconv"
)

// SetupRedirect installs OUTPUT nat redirect for DNS (udp/tcp 53 -> port).
// Requires CAP_NET_ADMIN inside the namespace; no root user needed.
func SetupRedirect(port int) error {
	targetPort := strconv.Itoa(port)
	rules := [][]string{
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", targetPort},
		{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", targetPort},
	}
	for _, args := range rules {
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables command failed: %v (output: %s)", err, output)
		}
	}
	return nil
}
