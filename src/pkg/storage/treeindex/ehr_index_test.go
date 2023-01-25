package treeindex

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/bsn-si/IPEHR-gateway/src/pkg/docs/model"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/docs/model/base"
	"github.com/bsn-si/IPEHR-gateway/src/pkg/errors"

	"github.com/stretchr/testify/assert"
)

func TestEHRIndex_AddEHR(t *testing.T) {
	tests := []struct {
		name    string
		getEHR  func() (model.EHR, error)
		want    map[string]*EHRNode
		wantErr bool
	}{
		{
			"1. success add new EHR object",
			func() (model.EHR, error) {
				ehr, err := loadEHRFromFile("./../../../../data/mock/ehr/ehr.json")
				if err != nil {
					return model.EHR{}, err
				}

				return ehr, nil
			},
			map[string]*EHRNode{
				"7d44b88c-4199-4bad-97dc-d78268e01398": {
					baseNode: baseNode{
						ID:   "7d44b88c-4199-4bad-97dc-d78268e01398",
						Type: base.EHRItemType,
					},
					attributes: map[string]Noder{
						"system_id": &ValueNode{
							data: "d60e2348-b083-48ce-93b9-916cef1d3a5a",
						},
						"ehr_id": &ValueNode{
							data: "7d44b88c-4199-4bad-97dc-d78268e01398",
						},
					},
					compositions: Container{},
				},
			},
			false,
		},
		{
			"2. EHR with simple Composition",
			func() (model.EHR, error) {
				ehr, err := loadEHRFromFile("./test_fixtures/ehr.json")
				if err != nil {
					return model.EHR{}, err
				}

				return ehr, nil
			},
			map[string]*EHRNode{
				"7d44b88c-4199-4bad-97dc-d78268e01398": {
					baseNode: baseNode{
						ID:   "7d44b88c-4199-4bad-97dc-d78268e01398",
						Type: base.EHRItemType,
					},
					attributes: map[string]Noder{
						"system_id": &ValueNode{
							data: "d60e2348-b083-48ce-93b9-916cef1d3a5a",
						},
						"ehr_id": &ValueNode{
							data: "7d44b88c-4199-4bad-97dc-d78268e01398",
						},
					},
					compositions: Container{
						"openEHR-EHR-COMPOSITION.health_summary.v1": []Noder{
							&CompositionNode{
								baseNode: baseNode{
									ID:   "openEHR-EHR-COMPOSITION.health_summary.v1",
									Type: base.CompositionItemType,
									Name: "International Patient Summary",
								},
								Tree: *NewTree(),
								attributes: map[string]Noder{
									"language": newNode(&base.CodePhrase{
										Type: base.CodePhraseItemType,
										TerminologyID: base.ObjectID{
											Type:  base.TerminologyIDItemType,
											Value: "ISO_639-1",
										},
										CodeString: "en",
									}),
									"territory": newNode(&base.CodePhrase{
										Type: base.CodePhraseItemType,
										TerminologyID: base.ObjectID{
											Type:  base.TerminologyIDItemType,
											Value: "ISO_3166-1",
										},
										CodeString: "US",
									}),
									"category": newNode(base.NewDvCodedText("event", base.CodePhrase{
										Type: base.CodePhraseItemType,
										TerminologyID: base.ObjectID{
											Type:  base.TerminologyIDItemType,
											Value: "openehr",
										},
										CodeString: "443",
									})),
								},
							},
						},
					},
				},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := NewEHRIndex()
			ehr, err := tt.getEHR()
			if err != nil {
				t.Errorf("EHRIndex.AddEHR(), cannot get EHR: %v", err)
				return
			}

			if err := idx.AddEHR(ehr); (err != nil) != tt.wantErr {
				t.Errorf("EHRIndex.AddEHR() error = %v, wantErr %v", err, tt.wantErr)
			}

			// dataWant, _ := json.Marshal(tt.want)
			// data, _ := json.Marshal(idx)

			// assert.Equal(t, string(dataWant), string(data))
			// got := map[string]any{}
			// _ = json.Unmarshal(data, &got)
			assert.Equal(t, tt.want, idx.ehrs)
		})
	}
}

func loadEHRFromFile(name string) (model.EHR, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return model.EHR{}, errors.Wrap(err, "cannot read file")
	}

	ehr := model.EHR{}
	if err := json.Unmarshal(data, &ehr); err != nil {
		return model.EHR{}, errors.Wrap(err, "cannot unmarshal EHR")
	}

	return ehr, nil
}
