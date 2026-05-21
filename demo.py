#!/usr/bin/env python3
"""
Interactive Router Demo
"""

from router import router


def main():
    """Interactive demo of the router"""
    print("=" * 70)
    print("Interactive Router Demo - Dispatch to bpmct")
    print("=" * 70)
    print()
    
    # Show available routes
    router.list_routes()
    
    # Interactive examples
    examples = [
        {
            'description': 'Basic bpmct dispatch',
            'path': '/bpmct',
            'kwargs': {'message': 'Hello from the router demo!'}
        },
        {
            'description': 'bpmct with action parameter',
            'path': '/bpmct/deploy',
            'kwargs': {}
        },
        {
            'description': 'bpmct with different action',
            'path': '/bpmct/restart',
            'kwargs': {}
        },
        {
            'description': 'bpmct API v1 endpoint',
            'path': '/api/v1/bpmct/users',
            'kwargs': {}
        },
        {
            'description': 'bpmct API v2 endpoint',
            'path': '/api/v2/bpmct/config',
            'kwargs': {}
        },
    ]
    
    print("=== Running Examples ===\n")
    
    for i, example in enumerate(examples, 1):
        print(f"{i}. {example['description']}")
        print(f"   Path: {example['path']}")
        try:
            result = router.dispatch(example['path'], **example['kwargs'])
            print(f"   ✓ Success!")
            print(f"   Response: {result}")
        except ValueError as e:
            print(f"   ✗ Error: {e}")
        print()
    
    # Custom dispatch demonstration
    print("=== Custom Dispatch Example ===\n")
    print("Dispatching custom request to bpmct...")
    custom_result = router.dispatch(
        '/bpmct',
        message='This demonstrates dynamic routing to bpmct!',
        user='demo_user',
        timestamp='2026-05-21'
    )
    print(f"Result: {custom_result}\n")
    
    print("=" * 70)
    print("Demo completed successfully!")
    print("=" * 70)


if __name__ == "__main__":
    main()
