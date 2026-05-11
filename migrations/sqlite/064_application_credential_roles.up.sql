PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS application_credential_roles (
    application_credential_id TEXT NOT NULL REFERENCES application_credentials(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (application_credential_id, role_id)
);
