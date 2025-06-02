// Type extensions for Wails-generated models
// This file extends the auto-generated types to include fields that are added by Go's JSON marshaling

import { network } from '../../wailsjs/go/models';

// Extend HTTPResponse to include the printable field that Go's JSON marshaling automatically provides
declare module '../../wailsjs/go/models' {
  namespace network {
    interface HTTPRequest {
    }

    interface HTTPResponse {
      printable: string;
    }
  }
}

export {}; // Make this a module 