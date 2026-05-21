#!/usr/bin/env python3
"""
Simple Router Demo - Dispatches requests to registered handlers
"""

from typing import Callable, Dict, Any, Optional
from dataclasses import dataclass
import re


@dataclass
class Route:
    """Represents a route with a pattern and handler"""
    pattern: str
    handler: Callable
    name: str
    
    def matches(self, path: str) -> Optional[Dict[str, str]]:
        """Check if path matches this route pattern"""
        regex_pattern = re.sub(r':(\w+)', r'(?P<\1>[^/]+)', self.pattern)
        regex_pattern = f"^{regex_pattern}$"
        match = re.match(regex_pattern, path)
        if match:
            return match.groupdict()
        return None


class Router:
    """Main router class that handles dispatching"""
    
    def __init__(self):
        self.routes: list[Route] = []
        
    def register(self, pattern: str, name: str):
        """Decorator to register a route handler"""
        def decorator(handler: Callable):
            route = Route(pattern=pattern, handler=handler, name=name)
            self.routes.append(route)
            return handler
        return decorator
    
    def dispatch(self, path: str, **kwargs) -> Any:
        """Dispatch a request to the appropriate handler"""
        for route in self.routes:
            params = route.matches(path)
            if params is not None:
                print(f"✓ Routing '{path}' to handler '{route.name}'")
                return route.handler(params, **kwargs)
        
        raise ValueError(f"No route found for path: {path}")
    
    def list_routes(self):
        """List all registered routes"""
        print("\n=== Registered Routes ===")
        for route in self.routes:
            print(f"  {route.pattern} -> {route.name}")
        print()


# Create the main router instance
router = Router()


# Register route handlers
@router.register("/bpmct", name="bpmct_handler")
def handle_bpmct(params: Dict[str, str], **kwargs):
    """Handler for bpmct requests"""
    message = kwargs.get('message', 'Hello from bpmct!')
    return {
        'status': 'success',
        'handler': 'bpmct',
        'message': message,
        'params': params
    }


@router.register("/bpmct/:action", name="bpmct_action_handler")
def handle_bpmct_action(params: Dict[str, str], **kwargs):
    """Handler for bpmct actions"""
    action = params.get('action', 'unknown')
    return {
        'status': 'success',
        'handler': 'bpmct_action',
        'action': action,
        'message': f'Executing action: {action}',
        'params': params
    }


@router.register("/api/:version/bpmct/:resource", name="bpmct_api_handler")
def handle_bpmct_api(params: Dict[str, str], **kwargs):
    """Handler for versioned bpmct API requests"""
    version = params.get('version', 'v1')
    resource = params.get('resource', 'default')
    return {
        'status': 'success',
        'handler': 'bpmct_api',
        'api_version': version,
        'resource': resource,
        'message': f'API {version} - Resource: {resource}',
        'params': params
    }


@router.register("/home", name="home_handler")
def handle_home(params: Dict[str, str], **kwargs):
    """Handler for home route"""
    return {
        'status': 'success',
        'handler': 'home',
        'message': 'Welcome to the router demo!'
    }


if __name__ == "__main__":
    # Demo usage
    print("=" * 60)
    print("Router Demo - Dispatching to bpmct")
    print("=" * 60)
    
    router.list_routes()
    
    # Test various routes
    test_paths = [
        ("/home", {}),
        ("/bpmct", {'message': 'Dispatched to bpmct successfully!'}),
        ("/bpmct/deploy", {}),
        ("/bpmct/status", {}),
        ("/api/v1/bpmct/users", {}),
        ("/api/v2/bpmct/config", {}),
    ]
    
    print("=== Dispatching Requests ===\n")
    
    for path, kwargs in test_paths:
        try:
            result = router.dispatch(path, **kwargs)
            print(f"Result: {result}\n")
        except ValueError as e:
            print(f"Error: {e}\n")
    
    # Test non-existent route
    print("=== Testing Non-existent Route ===\n")
    try:
        router.dispatch("/nonexistent")
    except ValueError as e:
        print(f"✗ {e}\n")
    
    print("=" * 60)
