package badger

import (
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/google/uuid"
)

// ensureUID ensures a resource has a UID, generating one if needed.
func ensureUID(resource apiary.Resource) {
	if resource.GetUID() != "" {
		return
	}

	// Generate UID and set it directly using type assertion
	newUID := uuid.New().String()
	
	switch r := resource.(type) {
	case *apiary.AgentSpec:
		r.ObjectMeta.UID = newUID
	case *apiary.Drone:
		r.ObjectMeta.UID = newUID
	case *apiary.Hive:
		r.ObjectMeta.UID = newUID
	case *apiary.Cell:
		r.ObjectMeta.UID = newUID
	case *apiary.Session:
		r.ObjectMeta.UID = newUID
	case *apiary.Secret:
		r.ObjectMeta.UID = newUID
	}
}
