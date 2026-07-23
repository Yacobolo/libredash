package ir

import (
	"encoding/json"
	"fmt"
)

const CurrentDataStateTransportSchemaVersion int32 = 1

// EncodedDataStateTransport is the internal serialization result corresponding
// to the canonical TypeSpec VisualizationDataStateTransport contract.
type EncodedDataStateTransport struct {
	SchemaVersion int32  `json:"schemaVersion"`
	Encoding      string `json:"encoding"`
	Kind          string `json:"kind"`
	SpecRevision  string `json:"specRevision"`
	DataRevision  int64  `json:"dataRevision"`
	Generation    int64  `json:"generation"`
	Payload       string `json:"payload"`
}

// EncodeDataStateTransport keeps the potentially large data-state payload
// opaque to reactive browser signal graphs while retaining a small, closed,
// versioned header that can be validated before decoding.
func EncodeDataStateTransport(state VisualizationDataState) (EncodedDataStateTransport, error) {
	specRevision, dataRevision, generation, err := dataStateRevisions(state)
	if err != nil {
		return EncodedDataStateTransport{}, err
	}
	if dataRevision < 0 || generation < 0 {
		return EncodedDataStateTransport{}, fmt.Errorf("visualization data revision and generation must be non-negative")
	}
	kind, err := dataStateKind(state)
	if err != nil {
		return EncodedDataStateTransport{}, err
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return EncodedDataStateTransport{}, fmt.Errorf("encode visualization data-state transport payload: %w", err)
	}
	return EncodedDataStateTransport{
		SchemaVersion: CurrentDataStateTransportSchemaVersion,
		Encoding:      "json",
		Kind:          kind,
		SpecRevision:  specRevision,
		DataRevision:  dataRevision,
		Generation:    generation,
		Payload:       string(payload),
	}, nil
}

func dataStateKind(state VisualizationDataState) (string, error) {
	return state.Kind()
}
