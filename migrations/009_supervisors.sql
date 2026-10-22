CREATE TABLE IF NOT EXISTS supervisors (
    id               SERIAL       PRIMARY KEY,
    name             VARCHAR(120) NOT NULL,
    quota            INTEGER      NOT NULL CHECK (quota > 0),
    supervised_count INTEGER      NOT NULL DEFAULT 0,
    version          INTEGER      NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT supervisors_supervised_within_quota
        CHECK (supervised_count >= 0 AND supervised_count <= quota)
);

CREATE TABLE IF NOT EXISTS supervisions (
    student_id    INTEGER     PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    supervisor_id INTEGER     NOT NULL REFERENCES supervisors(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS supervisions_supervisor_id_idx ON supervisions (supervisor_id);

INSERT INTO supervisors (id, name, quota, supervised_count, version) VALUES
    (1, 'Dr. Ir. Budi Santoso, M.Kom', 5, 0, 1),
    (2, 'Prof. Siti Aminah, Ph.D',     3, 0, 1),
    (3, 'Ahmad Fauzi, M.T.',           4, 0, 1),
    (4, 'Dian Pratama, M.Cs',          3, 0, 1),
    (5, 'Eko Wijaya, Ph.D',            2, 0, 1)
ON CONFLICT (id) DO NOTHING;

SELECT setval('supervisors_id_seq', (SELECT COALESCE(MAX(id), 1) FROM supervisors));

INSERT INTO permissions (name, description) VALUES
    ('supervision:manage', 'Menetapkan dan mengelola dosen pembimbing tugas akhir')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_name, permission_name) VALUES
    ('admin', 'supervision:manage')
ON CONFLICT DO NOTHING;
