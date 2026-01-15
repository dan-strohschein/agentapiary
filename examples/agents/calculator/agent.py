#!/usr/bin/env python3
"""
Calculator agent that performs mathematical operations.

This agent demonstrates:
- Parsing structured input
- Performing computations
- Returning structured JSON responses
"""

import sys
import json
import time
import re

def calculate(expression):
    """
    Safely evaluate a mathematical expression.
    Only allows numbers, operators, and parentheses.
    """
    # Sanitize input - only allow safe characters
    if not re.match(r'^[0-9+\-*/().\s]+$', expression):
        raise ValueError(f"Invalid expression: {expression}")
    
    try:
        # Use eval in a controlled way (for demo - in production, use ast.literal_eval or a parser)
        result = eval(expression)
        return float(result) if isinstance(result, (int, float)) else result
    except Exception as e:
        raise ValueError(f"Calculation error: {e}")

def main():
    """Main agent loop - reads calculations and returns results."""
    try:
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            
            try:
                # Try to parse as JSON
                message = json.loads(line)
                message_type = message.get('type', 'unknown')
                
                if message_type == 'calculation':
                    expression = message.get('expression', '')
                elif message_type == 'message':
                    # Extract expression from message content
                    content = message.get('content', '')
                    # Try to find mathematical expression
                    expression = content.strip()
                else:
                    expression = message.get('content', line)
                
                # Perform calculation
                result = calculate(expression)
                
                # Create structured response
                response = {
                    'type': 'calculation_result',
                    'expression': expression,
                    'result': result,
                    'timestamp': int(time.time())
                }
                
                print(json.dumps(response))
                sys.stdout.flush()
                
            except json.JSONDecodeError:
                # Not JSON, treat as plain expression
                try:
                    result = calculate(line)
                    response = {
                        'type': 'calculation_result',
                        'expression': line,
                        'result': result,
                        'timestamp': int(time.time())
                    }
                    print(json.dumps(response))
                    sys.stdout.flush()
                except ValueError as e:
                    error_response = {
                        'type': 'error',
                        'error': str(e),
                        'timestamp': int(time.time())
                    }
                    print(json.dumps(error_response), file=sys.stderr)
                    sys.stderr.flush()
                    
            except ValueError as e:
                error_response = {
                    'type': 'error',
                    'error': str(e),
                    'timestamp': int(time.time())
                }
                print(json.dumps(error_response), file=sys.stderr)
                sys.stderr.flush()
                
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
