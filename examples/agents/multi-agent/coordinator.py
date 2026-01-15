#!/usr/bin/env python3
"""
Multi-agent coordinator that orchestrates multiple sub-agents.

This agent demonstrates:
- Coordinating multiple agents in a hierarchical pattern
- Aggregating results
- Making decisions based on multiple inputs
"""

import sys
import json
import time
from typing import List, Dict, Any

class CoordinatorAgent:
    """Coordinates multiple sub-agents."""
    
    def __init__(self):
        self.agents = {
            'validator': self.validate_input,
            'processor': self.process_data,
            'formatter': self.format_output
        }
    
    def validate_input(self, data: Any) -> Dict[str, Any]:
        """Validate input data."""
        if isinstance(data, str) and len(data) > 0:
            return {'valid': True, 'length': len(data)}
        return {'valid': False, 'error': 'Invalid input'}
    
    def process_data(self, data: Any) -> Dict[str, Any]:
        """Process the data."""
        if isinstance(data, str):
            return {
                'processed': True,
                'uppercase': data.upper(),
                'lowercase': data.lower(),
                'word_count': len(data.split())
            }
        return {'processed': False, 'error': 'Cannot process non-string data'}
    
    def format_output(self, data: Dict[str, Any]) -> str:
        """Format the output."""
        return json.dumps(data, indent=2)
    
    def coordinate(self, input_data: Any) -> Dict[str, Any]:
        """Coordinate all sub-agents."""
        results = {}
        
        # Step 1: Validate
        validation = self.agents['validator'](input_data)
        results['validation'] = validation
        
        if not validation.get('valid', False):
            return {'error': 'Validation failed', 'details': validation}
        
        # Step 2: Process
        processing = self.agents['processor'](input_data)
        results['processing'] = processing
        
        # Step 3: Format
        formatted = self.agents['formatter'](results)
        results['formatted_output'] = formatted
        
        return results

def main():
    """Main agent loop."""
    coordinator = CoordinatorAgent()
    
    try:
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            
            try:
                # Parse input
                message = json.loads(line)
                message_type = message.get('type', 'unknown')
                
                if message_type == 'coordinate':
                    input_data = message.get('data', message.get('content', ''))
                else:
                    input_data = message.get('content', line)
                
                # Coordinate sub-agents
                result = coordinator.coordinate(input_data)
                
                # Create response
                response = {
                    'type': 'coordination_result',
                    'result': result,
                    'timestamp': int(time.time())
                }
                
                print(json.dumps(response))
                sys.stdout.flush()
                
            except json.JSONDecodeError:
                # Not JSON, treat as plain input
                result = coordinator.coordinate(line)
                response = {
                    'type': 'coordination_result',
                    'result': result,
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
