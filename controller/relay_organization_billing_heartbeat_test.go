package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/stretchr/testify/require"
)

// TestStartRelayOrganizationBillingHeartbeatRenewsSession covers the non-streaming
// gap: long upstream calls never touch the billing session, so the repair task
// refunds it as an orphan once the session TTL expires. The relay-level helper
// must keep the session alive for the whole request.
func TestStartRelayOrganizationBillingHeartbeatRenewsSession(t *testing.T) {
	setupControllerOrganizationTestDB(t)

	previousStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 2 // session TTL 4s, heartbeat interval 2s
	t.Cleanup(func() { constant.StreamingTimeout = previousStreamingTimeout })

	now := common.GetTimestamp()
	session := model.OrganizationBillingSession{
		OrganizationId:   1,
		IdempotencyKey:   "relay-heartbeat-nonstream",
		Status:           model.OrganizationBillingSessionStatusPreConsumed,
		PreConsumedQuota: 100,
		LastHeartbeatAt:  now,
		ExpiresAt:        now + 4,
	}
	require.NoError(t, model.DB.Create(&session).Error)

	relayInfo := &relaycommon.RelayInfo{
		OrganizationBillingSessionId:  session.Id,
		OrganizationBillingSessionKey: session.IdempotencyKey,
	}

	stop := startRelayOrganizationBillingHeartbeat(relayInfo)
	defer stop()

	require.Eventually(t, func() bool {
		var stored model.OrganizationBillingSession
		if err := model.DB.First(&stored, session.Id).Error; err != nil {
			return false
		}
		return stored.ExpiresAt > session.ExpiresAt && stored.LastHeartbeatAt >= session.LastHeartbeatAt
	}, 5*time.Second, 100*time.Millisecond)
}
