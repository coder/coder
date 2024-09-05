CREATE TABLE frobulators
(
    id           uuid NOT NULL,
    user_id      uuid NOT NULL,
    org_id       uuid NOT NULL,
    model_number TEXT NOT NULL,
    PRIMARY KEY (id),
    UNIQUE (model_number),
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE
);
