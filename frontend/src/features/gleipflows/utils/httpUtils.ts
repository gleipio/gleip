import { network } from '../../../../wailsjs/go/models';

/**
 * Generate a raw HTTP request from method, URL, headers, and body
 */
export function generateRawHttpRequest(
  method: string,
  url: string,
  headers: Record<string, string>,
  body: string
): string {
  // Extract path from URL if it's a full URL
  let path = url;
  try {
    if (url.includes('://')) {
      const urlObj = new URL(url);
      path = urlObj.pathname + urlObj.search;
    }
  } catch (e) {
    // If URL parsing fails, use the original URL
    console.error('Failed to parse URL', url, e);
  }

  // Build request line
  let rawRequest = `${method} ${path} HTTP/1.1\r\n`;

  // Add headers
  for (const [key, value] of Object.entries(headers)) {
    rawRequest += `${key}: ${value}\r\n`;
  }

  // Add host header if missing but URL has it
  if (!headers['Host'] && !headers['host'] && url.includes('://')) {
    try {
      const urlObj = new URL(url);
      rawRequest += `Host: ${urlObj.host}\r\n`;
    } catch (e) {
      // If URL parsing fails, try to extract host manually
      const hostMatch = url.match(/https?:\/\/([^\/]+)/);
      if (hostMatch && hostMatch[1]) {
        rawRequest += `Host: ${hostMatch[1]}\r\n`;
      }
    }
  }

  // Add body if present
  if (body) {
    // Add Content-Length if not present
    if (!headers['Content-Length'] && !headers['content-length']) {
      rawRequest += `Content-Length: ${new TextEncoder().encode(body).length}\r\n`;
    }
    rawRequest += `\r\n${body}`;
  } else {
    rawRequest += '\r\n';
  }

  return rawRequest;
}

/**
 * Parse a raw HTTP request into its components
 */
export function parseRawHttpRequest(rawRequest: string): network.HTTPRequest {
  // Extract host from Host header in the raw request
  const lines = rawRequest.split('\r\n');
  let host = '';
  let tls = false;
  
  // Look for Host header
  for (const line of lines) {
    if (line.toLowerCase().startsWith('host:')) {
      host = line.substring(5).trim();
      break;
    }
  }
  
  // Determine TLS based on common patterns or explicit indicators
  // This is a best guess since raw requests don't contain the protocol
  tls = host.includes(':443') || rawRequest.toLowerCase().includes('upgrade-insecure-requests');

  return {
    host: host,
    tls: tls,
    dump: rawRequest
  } as network.HTTPRequest;
}

/**
 * Apply syntax highlighting to HTTP request
 */
export function highlightHttpSyntax(httpText: string): string {
  if (!httpText) return '';
  
  const lines = httpText.split(/\r\n|\n|\r/);
  let highlightedHtml = '';
  
  // Process the first line (request/status line)
  if (lines.length > 0) {
    const firstLine = lines[0];
    const parts = firstLine.split(' ');
    
    if (parts.length >= 3) {
      // Is it a request?
      if (/^[A-Z]+$/.test(parts[0])) {
        highlightedHtml += `<span class="http-method">${parts[0]}</span> `;
        highlightedHtml += `<span class="http-path">${parts[1]}</span> `;
        highlightedHtml += `<span class="http-version">${parts.slice(2).join(' ')}</span>\n`;
      }
      // Is it a response?
      else if (parts[0].startsWith('HTTP/')) {
        highlightedHtml += `<span class="http-version">${parts[0]}</span> `;
        highlightedHtml += `<span class="http-status-code">${parts[1]}</span> `;
        highlightedHtml += `<span class="http-status-text">${parts.slice(2).join(' ')}</span>\n`;
      }
      else {
        highlightedHtml += `${firstLine}\n`;
      }
    } else {
      highlightedHtml += `${firstLine}\n`;
    }
  }
  
  // Process headers and body
  let inBody = false;
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i];
    
    // Empty line indicates the start of the body
    if (line.trim() === '') {
      inBody = true;
      highlightedHtml += '\n';
      continue;
    }
    
    if (!inBody) {
      // Process header line
      const colonIndex = line.indexOf(':');
      if (colonIndex > 0) {
        const key = line.substring(0, colonIndex);
        const value = line.substring(colonIndex + 1);
        
        highlightedHtml += `<span class="http-header-key">${key}</span>: `;
        highlightedHtml += `<span class="http-header-value">${value}</span>\n`;
      } else {
        highlightedHtml += `${line}\n`;
      }
    } else {
      // Process body line
      highlightedHtml += `<span class="http-body">${line}</span>\n`;
    }
  }
  
  return highlightedHtml;
}
