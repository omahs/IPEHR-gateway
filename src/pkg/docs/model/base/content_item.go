package base

// ContentItem
// Abstract ancestor of all concrete content types.
// https://specifications.openehr.org/releases/RM/Release-1.0.2/ehr.html#_content_item_class
type ContentItem struct {
	Locatable
}

type ItemType string

func (cit ItemType) ToString() string {
	return string(cit)
}

const (
	ActionItemType          ItemType = "ACTION"
	ActivityItemType        ItemType = "ACTIVITY"
	ArchetypedItemType      ItemType = "ARCHETYPED"
	ArchetypeIDItemType     ItemType = "ARCHETYPE_ID"
	ClusterItemType         ItemType = "CLUSTER"
	CodePhraseItemType      ItemType = "CODE_PHRASE"
	CompositionItemType     ItemType = "COMPOSITION"
	DvBooleanItemType       ItemType = "DV_BOOLEAN"
	DvCodedTextItemType     ItemType = "DV_CODED_TEXT"
	DvCountItemType         ItemType = "DV_COUNT"
	DvDateItemType          ItemType = "DV_DATE"
	DvDateTimeItemType      ItemType = "DV_DATE_TIME"
	DvDurationItemType      ItemType = "DV_DURATION"
	DvIdentifierItemType    ItemType = "DV_IDENTIFIER"
	DvIntervalItemType      ItemType = "DV_INTERVAL"
	DvMultimediaItemType    ItemType = "DV_MULTIMEDIA"
	DvOrderedItemType       ItemType = "DV_ORDERED"
	DvParagraphItemType     ItemType = "DV_PARAGRAPH"
	DvParsableItemType      ItemType = "DV_PARSABLE"
	DvProportionItemType    ItemType = "DV_PROPORTION"
	DvStateItemType         ItemType = "DV_STATE"
	DvQuantityItemType      ItemType = "DV_QUANTITY"
	DvTextItemType          ItemType = "DV_TEXT"
	DvTimeItemType          ItemType = "DV_TIME"
	DvURIItemType           ItemType = "DV_URI"
	EHRAccessItemType       ItemType = "EHR_ACCESS"
	EHRStatusItemType       ItemType = "EHR_STATUS"
	ElementItemType         ItemType = "ELEMENT"
	EvaluationItemType      ItemType = "EVALUATION"
	EventContextItemType    ItemType = "EVENT_CONTEXT"
	GenericIDItemType       ItemType = "GENERIC_ID"
	HierObjectIDItemType    ItemType = "HIER_OBJECT_ID"
	HistoryItemType         ItemType = "HISTORY"
	InstructionItemType     ItemType = "INSTRUCTION"
	IsmTransitionItemType   ItemType = "ISM_TRANSITION"
	ItemSingleItemType      ItemType = "ITEM_SINGLE"
	ItemListItemType        ItemType = "ITEM_LIST"
	ItemTableItemType       ItemType = "ITEM_TABLE"
	ItemTreeItemType        ItemType = "ITEM_TREE"
	ObjectVersionIDItemType ItemType = "OBJECT_VERSION_ID"
	ObservationItemType     ItemType = "OBSERVATION"
	ParticipationItemType   ItemType = "PARTICIPATION"
	PartyIdentifiedItemType ItemType = "PARTY_IDENTIFIED"
	PartyRefItemType        ItemType = "PARTY_REF"
	PartySelfItemType       ItemType = "PARTY_SELF"
	PointEventItemType      ItemType = "POINT_EVENT"
	IntervalEventItemType   ItemType = "INTERVAL_EVENT"
	SectionItemType         ItemType = "SECTION"
	TemplateIDItemType      ItemType = "TEMPLATE_ID"
	TerminologyIDItemType   ItemType = "TERMINOLOGY_ID"
)
