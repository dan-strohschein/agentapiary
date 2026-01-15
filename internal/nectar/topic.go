package nectar

import (
	"strings"
)

// PrefixTopic prefixes a topic with the Cell (namespace) name to ensure isolation.
// Format: cell.{namespace}.{originalTopic}
// Optimized: Uses strings.Builder to avoid fmt.Sprintf overhead.
func PrefixTopic(namespace, topic string) string {
	if namespace == "" {
		// Empty namespace means no prefix (for system-level topics)
		return topic
	}
	var b strings.Builder
	b.Grow(len(namespace) + len(topic) + 6) // "cell." + namespace + "." + topic
	b.WriteString("cell.")
	b.WriteString(namespace)
	b.WriteByte('.')
	b.WriteString(topic)
	return b.String()
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
