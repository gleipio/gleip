import { backend, network } from '../../../wailsjs/go/models';

export type StepAttributes = {
  id: string;
  name: string;
  isExpanded: boolean;
};

// Types for request gleip
export type RequestStep = {
  stepAttributes: StepAttributes;
  request: network.HTTPRequest; // This contains the raw HTTP dump and metadata
  response: network.HTTPResponse;
  variableExtracts: VariableExtract[];
  recalculateContentLength: boolean;
  gunzipResponse: boolean;
  fuzzSettings?: FuzzSettings; // Optional fuzz settings
  isFuzzMode: boolean; // Whether the step is in fuzz mode vs parse mode
  cameFrom: string; // Source of the request step: "history", "intercept", "clipboard", "user"
};

export type FuzzSettings = {
  delay: number; // Delay between requests in seconds
  currentWordlist: string[]; // List of words to fuzz with
  fuzzResults: FuzzResult[]; // Results of fuzzing
};

export type FuzzResult = {
  word: string; // The word that was used in the fuzz
  request: string; // The raw request that was sent
  response: string; // The raw response that was received
  statusCode: number; // HTTP status code
  size: number; // Size of response in bytes
  time: number; // Time taken in milliseconds
};

export type ScriptStep = {
  stepAttributes: StepAttributes;
  content: string;
};

export type ChefAction = {
  id: string;
  actionType: string;
  options: Record<string, any>;
  preview: string;
};

export type ChefStep = {
  stepAttributes: StepAttributes;
  inputVariable: string;
  actions: ChefAction[];
  outputVariable: string;
};

export type VariableExtract = {
  name: string;
  source: string;
  selector: string;
};

export type PhantomRequest = {
  host: string;
  tls: boolean;
  dump: string;
};

export type GleipFlowStep = {
  stepType: string;
  selected: boolean; // Flag to indicate if this step should be executed
  requestStep?: RequestStep;
  scriptStep?: ScriptStep;
  chefStep?: ChefStep;
  variablesStep?: Record<string, string>; // Needed for displaying variables as a step in the UI
};

export type GleipFlow = {
  id: string;
  name: string;
  variables: Record<string, string>;
  steps: GleipFlowStep[];
  sortingIndex: number; // Index for tab ordering (1 to n)
  executionResults?: ExecutionResult[]; // Execution results from the backend
  isVariableStepExpanded?: boolean; // Expansion state for variables step
  cachedPhantomRequests?: PhantomRequest[]; // Cached suggested requests
};

export type ExecutionResult = backend.ExecutionResult & {
  actualRawRequest?: string; // Actual raw request text that was sent
};

// Type for real-time step execution event
export type StepExecutionEvent = {
  gleipId: string;
  currentStepIndex: number;
  results: ExecutionResult[];
}; 