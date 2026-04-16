package trust

// ActionClass is the constitutional category of a consequential action.
type ActionClass string

const (
	ActionReadOnlyInspection      ActionClass = "read_only_inspection"
	ActionDerivedMirrorGeneration ActionClass = "derived_mirror_generation"
	ActionCandidateGeneration     ActionClass = "candidate_generation"
	ActionPRCreation              ActionClass = "pr_creation"
	ActionOperationalMutation     ActionClass = "operational_state_mutation"
	ActionCanonicalMemoryMutation ActionClass = "canonical_memory_mutation"
	ActionMissionStateMutation    ActionClass = "mission_state_mutation"
	ActionSessionLineageMutation  ActionClass = "session_lineage_mutation"
	ActionRouteOrStaffingChange   ActionClass = "route_or_staffing_change"
	ActionPolicyTuningChange      ActionClass = "policy_tuning_change"
	ActionCodeOrHarnessMutation   ActionClass = "code_or_harness_mutation"
	ActionDestructiveMutation     ActionClass = "destructive_archival_or_deletion"
)

// StateClass is the constitutional kind of state an action touches.
type StateClass string

const (
	StateConstitutional StateClass = "constitutional"
	StateOperational    StateClass = "operational"
	StateKnowledge      StateClass = "knowledge"
	StateReflective     StateClass = "reflective"
	StateDerived        StateClass = "derived"
	StateEvidentiary    StateClass = "evidentiary"
)

// DecisionKind is the trust gate's structured decision.
type DecisionKind string

const (
	DecisionAllowed               DecisionKind = "allowed"
	DecisionAllowedWithProof      DecisionKind = "allowed_with_proof"
	DecisionProposalRequired      DecisionKind = "proposal_required"
	DecisionApprovalRequired      DecisionKind = "approval_required"
	DecisionDeniedOfficeBoundary  DecisionKind = "denied_by_office_boundary"
	DecisionDeniedPromotionLaw    DecisionKind = "denied_by_promotion_law"
	DecisionUnknownClassification DecisionKind = "unknown_classification"
)

// Request describes a consequential action to classify.
type Request struct {
	ProducingOffice    string
	ProducingSubsystem string
	ActionClass        ActionClass
	TargetDomain       string
	TouchedState       []StateClass
	CurrentTrustStage  int
	HasPromotionPath   bool
	IsDestructive      bool
	RequestedAuthority string
}

// Response is the classifier's structured decision.
type Response struct {
	Decision              DecisionKind
	Reason                string
	RequiredProof         bool
	RequiredSignature     bool
	RequiredPromotionPath bool
	SuggestedTransformation string
}
