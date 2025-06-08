import { backend, network } from '../../../wailsjs/go/models';

// Types for request gleip
export type RequestStep = {
  id: string;
  name: string;
  request: network.HTTPRequest; // This contains the raw HTTP dump and metadata
  variableExtracts: VariableExtract[];
  recalculateContentLength: boolean;
  gunzipResponse: boolean;
  fuzzSettings?: FuzzSettings; // Optional fuzz settings
  isConfigExpanded: boolean; // Whether the configuration section is expanded
  isFuzzMode: boolean; // Whether the step is in fuzz mode vs parse mode
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
  id: string;
  name: string;
  content: string;
};

export type ChefAction = {
  id: string;
  actionType: string;
  options: Record<string, any>;
  preview: string;
};

export type ChefStep = {
  id: string;
  name: string;
  inputVariable: string;
  actions: ChefAction[];
  outputVariable: string;
};

export type VariableExtract = {
  name: string;
  source: string;
  selector: string;
};

export type GleipFlowStep = {
  stepType: string;
  requestStep?: RequestStep;
  scriptStep?: ScriptStep;
  chefStep?: ChefStep;
  variablesStep?: Record<string, string>;
  selected?: boolean; // Flag to indicate if this step should be executed
};

export type GleipFlow = {
  id: string;
  name: string;
  variables: Record<string, string>;
  steps: GleipFlowStep[];
  sortingIndex: number; // Index for tab ordering (1 to n)
  executionResults?: ExecutionResult[]; // Execution results from the backend
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