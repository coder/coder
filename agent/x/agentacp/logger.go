package agentacp

import (
	"context"
	stdslog "log/slog" //nolint:depguard // The ACP SDK requires a standard slog handler.

	"cdr.dev/slog/v3"
)

type slogHandler struct {
	logger slog.Logger
	group  string
}

func (*slogHandler) Enabled(context.Context, stdslog.Level) bool {
	// Coder's logger applies the configured level filter.
	return true
}

func (h *slogHandler) Handle(ctx context.Context, record stdslog.Record) error {
	fields := make([]slog.Field, 0, record.NumAttrs())
	record.Attrs(func(attr stdslog.Attr) bool {
		fields = appendLogAttr(fields, h.group, attr)
		return true
	})
	switch {
	case record.Level >= stdslog.LevelError:
		h.logger.Error(ctx, record.Message, fields...)
	case record.Level >= stdslog.LevelWarn:
		h.logger.Warn(ctx, record.Message, fields...)
	case record.Level >= stdslog.LevelInfo:
		h.logger.Info(ctx, record.Message, fields...)
	default:
		h.logger.Debug(ctx, record.Message, fields...)
	}
	return nil
}

func (h *slogHandler) WithAttrs(attrs []stdslog.Attr) stdslog.Handler {
	fields := make([]slog.Field, 0, len(attrs))
	for _, attr := range attrs {
		fields = appendLogAttr(fields, h.group, attr)
	}
	return &slogHandler{logger: h.logger.With(fields...), group: h.group}
}

func (h *slogHandler) WithGroup(name string) stdslog.Handler {
	if name == "" {
		return h
	}
	return &slogHandler{logger: h.logger, group: h.group + name + "_"}
}

func appendLogAttr(fields []slog.Field, group string, attr stdslog.Attr) []slog.Field {
	// Raw protocol frames can contain workspace secrets or conversation content.
	if attr.Key == "raw" {
		return fields
	}
	attr.Value = attr.Value.Resolve()
	if attr.Equal(stdslog.Attr{}) {
		return fields
	}
	if attr.Value.Kind() == stdslog.KindGroup {
		if attr.Key != "" {
			group += attr.Key + "_"
		}
		for _, child := range attr.Value.Group() {
			fields = appendLogAttr(fields, group, child)
		}
		return fields
	}
	return append(fields, slog.F(group+attr.Key, attr.Value.Any()))
}
