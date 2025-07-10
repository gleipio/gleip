import { ClipboardGetText } from '../../../../wailsjs/runtime/runtime';
import { parseRawHttpRequest } from './httpUtils';
import { network } from '../../../../wailsjs/go/models';
import { CreateHTTPRequest } from '../../../../wailsjs/go/backend/App';
import type { RequestStep } from '../types';

/**
 * Check if an object is an HTTPRequest format
 */
function isHTTPRequest(obj: any): obj is network.HTTPRequest {
  return obj && typeof obj.dump === 'string' && typeof obj.host === 'string' && 
         typeof obj.tls === 'boolean';
}

/**
 * Try to get a request from the clipboard
 */
export async function getRequestFromClipboard(): Promise<{
  success: boolean;
  request?: network.HTTPRequest;
  message: string;
}> {
  try {
    // Read from clipboard using Wails runtime
    const clipboardText = await ClipboardGetText();
    
    // First try to parse as JSON
    try {
      const pastedData = JSON.parse(clipboardText);
      
      // Check if it's the new HTTPRequest format
      if (isHTTPRequest(pastedData)) {
        const request = await CreateHTTPRequest(pastedData.host, pastedData.tls, pastedData.dump);
        return {
          success: true,
          request: request,
          message: 'Request parsed from HTTPRequest format'
        };
      }
      
      // Check if it's the old ClipboardRequest format
      if (isHTTPRequest(pastedData)) {
        const request = await CreateHTTPRequest(pastedData.host, pastedData.tls, pastedData.dump);
        return {
          success: true,
          request: request,
          message: 'Request parsed from ClipboardRequest format'
        };
      }
      
      // Not a recognized format - try to parse as raw HTTP
      return await tryParseRawHttp(clipboardText);
    } catch (e) {
      // Not valid JSON - try to parse as raw HTTP
      return await tryParseRawHttp(clipboardText);
    }
  } catch (e) {
    return {
      success: false,
      message: 'Failed to access clipboard'
    };
  }
}

/**
 * Try to parse a raw HTTP request from text
 */
async function tryParseRawHttp(text: string): Promise<{
  success: boolean;
  request?: network.HTTPRequest;
  message: string;
}> {
  try {
    // Check if it looks like an HTTP request (starts with method and path)
    if (/^[A-Z]+ [^ ]+ HTTP\//.test(text)) {
      const { tls, host, dump } = parseRawHttpRequest(text);
      
      const request = await CreateHTTPRequest(host, tls, dump);
      
      return {
        success: true,
        request,
        message: 'Request parsed from raw HTTP'
      };
    } else {
      return {
        success: false,
        message: 'Clipboard does not contain a valid request'
      };
    }
  } catch (e) {
    return {
      success: false,
      message: 'Failed to parse clipboard content'
    };
  }
}

/**
 * Create a new flow step from clipboard request data
 */
export function createRequestStepFromClipboard(request: network.HTTPRequest): RequestStep {
  const uniqueId = crypto.randomUUID();

  return {
      stepAttributes: {
        id: uniqueId,
        name: `Request`,
        isExpanded: false
      },
      request: request as network.HTTPRequest,
      response: { dump: '' } as network.HTTPResponse,
      variableExtracts: [],
      recalculateContentLength: true,
      gunzipResponse: true,
      isFuzzMode: false,
      cameFrom: "clipboard",
    }
} 