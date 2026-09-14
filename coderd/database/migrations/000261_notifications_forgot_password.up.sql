INSERT INTO notification_templates (id, name, title_template, body_template, "group", actions)
VALUES ('62f86a30-2330-4b61-a26d-311ff3b608cf', 'One-Time Passcode', E'Your One-Time Passcode for Coder.',
        E'Hi {{.UserName}},\n\nA request to reset the password for your Coder account has been made. Your one-time passcode is:\n\n**{{.Labels.one_time_passcode}}**\n\nIf you did not request to reset your password, you can ignore this message.',
        'User Events', '[]'::jsonb);
