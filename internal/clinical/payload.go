package clinical

import (
	"encoding/json"
	"fmt"
)

// PayloadVersion is the current version of the stored encounter payload.
//
//	1 (or absent): the original payload. Older model text could call the user
//	  "the patient", which was rewritten each time it was displayed.
//	2: that wording is rewritten once, when the payload is read, and display
//	  no longer changes stored text.
const PayloadVersion = 2

// migrations upgrade a payload from the version before each key to that version.
var migrations = map[int]func(*Encounter){
	2: migrateWording,
}

// DecodeEncounter reads a stored payload and migrates it to PayloadVersion.
func DecodeEncounter(b []byte) (*Encounter, error) {
	var e Encounter
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	if e.PayloadVersion > PayloadVersion {
		return nil, fmt.Errorf("this record was saved by a newer version of healthctl (payload version %d)", e.PayloadVersion)
	}
	for v := max(e.PayloadVersion, 1) + 1; v <= PayloadVersion; v++ {
		migrations[v](&e)
		e.PayloadVersion = v
	}
	return &e, nil
}
