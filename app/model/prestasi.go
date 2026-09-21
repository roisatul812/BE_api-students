package model

type Prestasi struct {
	ID           int    `json:"id"`
	NamaPrestasi string `json:"nama_prestasi"`
	StudentID    int    `json:"student_id"`
	Juara        string `json:"juara"`
}
