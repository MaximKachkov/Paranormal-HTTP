package model

type DossierInput struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ThreatLevel     int      `json:"threat_level"`
	Vulnerabilities []string `json:"vulnerabilities"`
}

type Dossier struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ThreatLevel     int      `json:"threat_level"`
	Vulnerabilities []string `json:"vulnerabilities"`
	EvidenceURLs    []string `json:"evidence_urls"`
}
