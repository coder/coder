# Router Demo - Dispatch to bpmct

A simple but powerful routing system that demonstrates dispatching requests to various handlers, with a focus on routing to bpmct.

## Features

- **Pattern Matching**: Supports parameterized routes (e.g., `/bpmct/:action`)
- **Dynamic Dispatch**: Routes requests to appropriate handlers based on path patterns
- **Extensible**: Easy to add new routes and handlers
- **Type-Safe**: Uses Python type hints for better code quality

## Project Structure

```
.
├── router.py    # Core router implementation with bpmct handlers
├── demo.py      # Interactive demonstration script
└── README.md    # This file
```

## Quick Start

### Run the Router Demo

```bash
python3 router.py
```

This will demonstrate the router dispatching to various bpmct endpoints.

### Run the Interactive Demo

```bash
python3 demo.py
```

This provides an interactive walkthrough of the routing capabilities.

## Available Routes

The demo includes the following routes that dispatch to bpmct:

| Pattern | Description | Example |
|---------|-------------|---------|
| `/bpmct` | Basic bpmct handler | `/bpmct` |
| `/bpmct/:action` | bpmct action handler | `/bpmct/deploy` |
| `/api/:version/bpmct/:resource` | Versioned API handler | `/api/v1/bpmct/users` |
| `/home` | Home route (for comparison) | `/home` |

## Usage Examples

### Basic Dispatch

```python
from router import router

# Dispatch to bpmct
result = router.dispatch('/bpmct', message='Hello!')
print(result)
# Output: {'status': 'success', 'handler': 'bpmct', 'message': 'Hello!', ...}
```

### Parameterized Routes

```python
# Dispatch with action parameter
result = router.dispatch('/bpmct/deploy')
print(result)
# Output: {'status': 'success', 'handler': 'bpmct_action', 'action': 'deploy', ...}
```

### API Routes

```python
# Dispatch to versioned API
result = router.dispatch('/api/v1/bpmct/users')
print(result)
# Output: {'status': 'success', 'handler': 'bpmct_api', 'api_version': 'v1', ...}
```

## Adding New Routes

To add a new route, use the `@router.register` decorator:

```python
@router.register("/bpmct/custom/:id", name="custom_handler")
def handle_custom(params, **kwargs):
    custom_id = params.get('id')
    return {
        'status': 'success',
        'id': custom_id,
        'message': f'Handling custom request for {custom_id}'
    }
```

## Architecture

### Router Class

The `Router` class is the core of the system:

- **register**: Decorator to register route handlers
- **dispatch**: Routes requests to the appropriate handler
- **list_routes**: Lists all registered routes

### Route Class

Each route consists of:

- **pattern**: URL pattern (supports `:param` syntax)
- **handler**: Function to handle matching requests
- **name**: Descriptive name for the route

## Requirements

- Python 3.7+
- No external dependencies (uses only standard library)

## Testing

Run the built-in tests:

```bash
python3 router.py
```

The output will show successful dispatching to all bpmct routes.

## License

MIT License
