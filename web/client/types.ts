export type * from "./src/types.gen";

// Preserve the acronym spelling used by earlier generated clients so UI and
// third-party callers can upgrade without a source-level breaking change.
export type {
  CallForwardingRuleJson as CallForwardingRuleJSON,
  CallJson as CallJSON,
  EventJson as EventJSON,
} from "./src/types.gen";
