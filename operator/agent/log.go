package agent

import (
	"aaa/operator/replication"
	"log"
)

func logAgentReceive(change *replication.ChangeEnvelope) {
	if change == nil {
		return
	}
	log.Printf("%s receive %s hostname=%s bytes=%d", agentLogPrefix, agentEventLabel(change.Type), change.Hostname, len(change.Bin))
}

func logAgentApply(change *replication.ChangeEnvelope, target string, err error) {
	if change == nil {
		return
	}
	if err != nil {
		log.Printf("%s %sapply%s %s hostname=%s target=%s err=%v", agentLogPrefix, agentLogFail, agentLogReset, agentEventLabel(change.Type), change.Hostname, target, err)
		return
	}
	log.Printf("%s %sapply%s %s hostname=%s target=%s", agentLogPrefix, agentLogOK, agentLogReset, agentEventLabel(change.Type), change.Hostname, target)
}

func agentEventLabel(kind replication.EventType) string {
	switch kind {
	case replication.EventType_EVENT_TYPE_DNS:
		return "\033[38;5;117m[DNS]\033[0m"
	case replication.EventType_EVENT_TYPE_TLS:
		return "\033[38;5;183m[TLS]\033[0m"
	case replication.EventType_EVENT_TYPE_HTTP:
		return "\033[38;5;229m[HTTP]\033[0m"
	default:
		return kind.String()
	}
}
