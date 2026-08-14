package platform

import (
	"testing"
	"time"
)

func TestSecurityAuditRecordCapturesActorAndTarget(t *testing.T) {
	log := &AuditLog{}
	log.AddSecurity(SecurityAuditRecord{ActorType: "user", ActorID: "user-1", SessionID: "session-1", Endpoint: "POST /tasks", TargetType: "task", TargetID: "task-1", Result: "success", At: time.Now()})
	if len(log.Records) != 1 || log.Records[0].Actor != "user:user-1" || log.Records[0].Target != "task:task-1" {
		t.Fatalf("security audit record was not persisted: %#v", log.Records)
	}
}
