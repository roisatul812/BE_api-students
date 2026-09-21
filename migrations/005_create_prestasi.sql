CREATE TABLE IF NOT EXISTS prestasi (
    id SERIAL PRIMARY KEY,
    nama_prestasi VARCHAR(150) NOT NULL,
    student_id INTEGER NOT NULL,
    juara VARCHAR(50) NOT NULL,
    CONSTRAINT prestasi_student_id_fkey
        FOREIGN KEY (student_id)
        REFERENCES students(id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS prestasi_student_id_idx
ON prestasi(student_id);