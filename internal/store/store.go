// Package store provides the storage abstraction for Apiary resources.
package store

import (
	"github.com/agentapiary/apiary/pkg/apiary"
)

// Store is the main interface for resource storage.
// This is an alias for apiary.ResourceStore to allow for future extensions.
type Store = apiary.ResourceStore
