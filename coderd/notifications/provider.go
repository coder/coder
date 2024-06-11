package notifications

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

type HandlerRegistry struct {
	handlers map[database.NotificationMethod]Handler
}

func NewHandlerRegistry(handlers ...Handler) (*HandlerRegistry, error) {
	reg := &HandlerRegistry{
		handlers: make(map[database.NotificationMethod]Handler),
	}

	for _, h := range handlers {
		if err := reg.Register(h); err != nil {
			return nil, err
		}
	}

	return reg, nil
}

func (p *HandlerRegistry) Register(handler Handler) error {
	method := handler.NotificationMethod()
	if _, found := p.handlers[method]; found {
		return xerrors.Errorf("%q already registered", method)
	}

	p.handlers[method] = handler
	return nil
}

func (p *HandlerRegistry) Resolve(method database.NotificationMethod) (Handler, error) {
	out, found := p.handlers[method]
	if !found {
		return nil, xerrors.Errorf("could not resolve handler by method %q", method)
	}

	return out, nil
}
