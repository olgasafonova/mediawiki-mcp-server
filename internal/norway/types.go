// Package norway provides a client for the Norwegian BRREG Enhetsregisteret API.
package norway

import "time"

// BRREGEnhet represents a company/entity from the BRREG API.
type BRREGEnhet struct {
	Organisasjonsnummer           string               `json:"organisasjonsnummer"`
	Navn                          string               `json:"navn"`
	Organisasjonsform             BRREGOrganisasjonsform `json:"organisasjonsform"`
	Hjemmeside                    string               `json:"hjemmeside,omitempty"`
	Registreringsdato             string               `json:"registreringsdatoEnhetsregisteret,omitempty"`
	Stiftelsesdato                string               `json:"stiftelsesdato,omitempty"`
	Oppstartsdato                 string               `json:"oppstartsdato,omitempty"`
	Slettedato                    string               `json:"slettedato,omitempty"`
	RegistrertIForetaksregisteret bool                 `json:"registrertIForetaksregisteret"`
	RegistrertIMvaregisteret      bool                 `json:"registrertIMvaregisteret"`
	RegistrertIStiftelsesregisteret bool               `json:"registrertIStiftelsesregisteret"`
	RegistrertIFrivillighetsregisteret bool            `json:"registrertIFrivillighetsregisteret"`
	Konkurs                       bool                 `json:"konkurs"`
	UnderAvvikling                bool                 `json:"underAvvikling"`
	UnderTvangsavviklingEllerTvangsopplosning bool    `json:"underTvangsavviklingEllerTvangsopplosning"`
	MaalformKode                  string               `json:"maalform,omitempty"`
	Naeringskode1                 *BRREGNaeringskode   `json:"naeringskode1,omitempty"`
	Naeringskode2                 *BRREGNaeringskode   `json:"naeringskode2,omitempty"`
	Naeringskode3                 *BRREGNaeringskode   `json:"naeringskode3,omitempty"`
	AntallAnsatte                 *int                 `json:"antallAnsatte,omitempty"`
	Forretningsadresse            *BRREGAdresse        `json:"forretningsadresse,omitempty"`
	Postadresse                   *BRREGAdresse        `json:"postadresse,omitempty"`
	InstitusjonellSektorkode      *BRREGSektorkode     `json:"institusjonellSektorkode,omitempty"`
	Overordnetenhet               string               `json:"overordnetEnhet,omitempty"`
	SisteInnsendteAarsregnskap    string               `json:"sisteInnsendteAarsregnskap,omitempty"`
	Links                         []BRREGLink          `json:"links,omitempty"`
}

// BRREGOrganisasjonsform represents the legal form of an entity.
type BRREGOrganisasjonsform struct {
	Kode        string `json:"kode"`
	Beskrivelse string `json:"beskrivelse"`
}

// BRREGNaeringskode represents an industry/business code (NACE).
type BRREGNaeringskode struct {
	Kode        string `json:"kode"`
	Beskrivelse string `json:"beskrivelse"`
}

// BRREGAdresse represents an address from BRREG.
type BRREGAdresse struct {
	Land          string   `json:"land,omitempty"`
	Landkode      string   `json:"landkode,omitempty"`
	Postnummer    string   `json:"postnummer,omitempty"`
	Poststed      string   `json:"poststed,omitempty"`
	Adresse       []string `json:"adresse,omitempty"`
	Kommune       string   `json:"kommune,omitempty"`
	Kommunenummer string   `json:"kommunenummer,omitempty"`
}

// BRREGSektorkode represents the institutional sector code.
type BRREGSektorkode struct {
	Kode        string `json:"kode"`
	Beskrivelse string `json:"beskrivelse"`
}

// BRREGLink represents a HATEOAS link.
type BRREGLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// BRREGSearchResponse represents search results from BRREG.
type BRREGSearchResponse struct {
	Embedded struct {
		Enheter []BRREGEnhet `json:"enheter"`
	} `json:"_embedded"`
	Page BRREGPage `json:"page"`
}

// BRREGPage represents pagination info.
type BRREGPage struct {
	Size          int `json:"size"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
	Number        int `json:"number"`
}

// BRREGRolle represents a role from the Rolle API.
type BRREGRolle struct {
	Type    BRREGRolleType `json:"type"`
	Person  *BRREGPerson   `json:"person,omitempty"`
	Enhet   *BRREGRolleEnhet `json:"enhet,omitempty"`
	Fratraadt bool          `json:"fratraadt"`
	Rekkefolge int          `json:"rekkefolge,omitempty"`
}

// BRREGRolleType represents the type of role.
type BRREGRolleType struct {
	Kode        string `json:"kode"`
	Beskrivelse string `json:"beskrivelse"`
}

// BRREGPerson represents a person from the Rolle API.
type BRREGPerson struct {
	Fodselsdato string `json:"fodselsdato,omitempty"`
	Navn        BRREGPersonNavn `json:"navn"`
	ErDod       bool   `json:"erDod"`
}

// BRREGPersonNavn represents a person's name.
type BRREGPersonNavn struct {
	Fornavn    string `json:"fornavn,omitempty"`
	Mellomnavn string `json:"mellomnavn,omitempty"`
	Etternavn  string `json:"etternavn,omitempty"`
}

// FullName returns the full name of a person.
func (n BRREGPersonNavn) FullName() string {
	name := n.Fornavn
	if n.Mellomnavn != "" {
		name += " " + n.Mellomnavn
	}
	if n.Etternavn != "" {
		name += " " + n.Etternavn
	}
	return name
}

// BRREGRolleEnhet represents a company holding a role.
type BRREGRolleEnhet struct {
	Organisasjonsnummer string `json:"organisasjonsnummer"`
	Organisasjonsnavn   string `json:"organisasjonsnavn,omitempty"`
}

// BRREGRollerResponse represents the roles response.
type BRREGRollerResponse struct {
	Rollegrupper []BRREGRolleGruppe `json:"rollegrupper"`
}

// BRREGRolleGruppe represents a group of roles.
type BRREGRolleGruppe struct {
	Type   BRREGRolleGruppeType `json:"type"`
	Roller []BRREGRolle         `json:"roller"`
}

// BRREGRolleGruppeType represents the type of role group.
type BRREGRolleGruppeType struct {
	Kode        string `json:"kode"`
	Beskrivelse string `json:"beskrivelse"`
}

// parseDate parses a BRREG date string (YYYY-MM-DD format).
func parseDate(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil
	}
	return &t
}
