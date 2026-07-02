-- Reword the autostop reminder to use a relative countdown instead of an
-- absolute timestamp.
UPDATE notification_templates SET body_template = E'Your workspace **{{.Labels.workspace}}** will automatically stop {{.Labels.timeTilShutdown}}.\n\nConnect to it or extend the deadline to keep it running.' WHERE id = '6f6cb984-c167-4fa5-bb87-1058dd642779';
