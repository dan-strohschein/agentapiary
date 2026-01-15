#!/usr/bin/env python3
"""
Simple echo agent that reads from stdin and echoes back with a prefix.

This agent demonstrates the stdin interface type.
It reads messages line-by-line and echoes them back.
"""

import sys
import json
import time

def main():
    """Main agent loop - reads from stdin and echoes back."""
    try:
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            
            # Try to parse as JSON (Waggle format)
            try:
                message = json.loads(line)
                message_type = message.get('type', 'unknown')
                content = message.get('content', line)
                
                # Create response
                response = {
                    'type': 'response',
                    'content': f'Echo: {content}',
                    'timestamp': int(time.time()),
                    'original_type': message_type
                }
                
                # Output response as JSON
                print(json.dumps(response))
                sys.stdout.flush()
            except json.JSONDecodeError:
                # Not JSON, just echo the line
                response = {
                    'type': 'response',
                    'content': f'Echo: {line}',
                    'timestamp': int(time.time())
                }
                print(json.dumps(response))
                sys.stdout.flush()
    except KeyboardInterrupt:
        pass
    except Exception as e:
        error_response = {
            'type': 'error',
            'error': str(e),
            'timestamp': int(time.time())
        }
        print(json.dumps(error_response), file=sys.stderr)
        sys.stderr.flush()

if __name__ == '__main__':
    main()
