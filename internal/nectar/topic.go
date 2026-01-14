package nectar

import (
	"fmt"
)

// PrefixTopic prefixes a topic with the Cell (namespace) name to ensure isolation.
// Format: cell.{namespace}.{originalTopic}
func PrefixTopic(namespace, topic string) string {
	if namespace == "" {
		// Empty namespace means no prefix (for system-level topics)
		return topic
	}
	return fmt.Sprintf("cell.%s.%s", namespace, topic)
}

// ExtractNamespace extracts the namespace from a prefixed topic.
// Returns the namespace and the remaining topic, or empty strings if not prefixed.
func ExtractNamespace(prefixedTopic string) (namespace, topic string) {
	prefix := "cell."
	if len(prefixedTopic) < len(prefix) || prefixedTopic[:len(prefix)] != prefix {
		return "", prefixedTopic
	}

	remaining := prefixedTopic[len(prefix):]
	// Find first dot after prefix
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == '.' {
			return remaining[:i], remaining[i+1:]
		}
	}
	return "", prefixedTopic
}
