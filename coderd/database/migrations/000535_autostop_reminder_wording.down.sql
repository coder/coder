-- Revert to the body introduced by migration 000528.
UPDATE notification_templates SET body_template = E'Your workspace **{{.Labels.workspace}}** is scheduled to automatically stop at {{.Labels.deadline}}.\n\nConnect to it or extend the deadline to keep it running.' WHERE id = '6f6cb984-c167-4fa5-bb87-1058dd642779';
