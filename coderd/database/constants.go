package database

import "github.com/google/uuid"

var PrebuildsSystemUserID = uuid.MustParse("c42fdf75-3097-471c-8c33-fb52454d81c0")

const (
	TailnetPeeringEventTypeAddedTunnel                 = "added_tunnel"
	TailnetPeeringEventTypeRemovedTunnel               = "removed_tunnel"
	TailnetPeeringEventTypePeerUpdateNode              = "peer_update_node"
	TailnetPeeringEventTypePeerUpdateDisconnected      = "peer_update_disconnected"
	TailnetPeeringEventTypePeerUpdateLost              = "peer_update_lost"
	TailnetPeeringEventTypePeerUpdateReadyForHandshake = "peer_update_ready_for_handshake"
)
