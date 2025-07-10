/**
 * Utility functions for HTTP request syntax highlighting and formatting
 */

/**
 * Extract host from a URL
 */
export function extractHostFromUrl(url: string): string {
  try {
    // Check if URL has protocol
    if (!url.includes('://')) {
      url = 'http://' + url;
    }
    
    const parsedUrl = new URL(url);
    return parsedUrl.host;
  } catch (e) {
    // Basic fallback if URL parsing fails
    if (url.includes('://')) {
      const parts = url.split('://');
      if (parts.length >= 2) {
        const hostPart = parts[1].split('/')[0];
        return hostPart;
      }
    } else if (url.includes('/')) {
      return url.split('/')[0];
    }
    return url;
  }
}

/**
 * Convert a request object to raw HTTP request syntax
 */
export function generateRawHttpRequest(
  method: string,
  url: string,
  headers: Record<string, string>,
  body: string
): string {
  // Extract path and host from URL
  let path = url;
  let host = '';
  
  try {
    // Add protocol if missing
    if (!url.includes('://')) {
      url = 'http://' + url;
    }
    
    const parsedUrl = new URL(url);
    path = parsedUrl.pathname + parsedUrl.search;
    host = parsedUrl.host;
  } catch (e) {
    // Basic fallback for invalid URLs
    if (url.includes('://')) {
      const parts = url.split('://');
      if (parts.length >= 2) {
        const afterProtocol = parts[1];
        const slashIndex = afterProtocol.indexOf('/');
        if (slashIndex >= 0) {
          host = afterProtocol.substring(0, slashIndex);
          path = afterProtocol.substring(slashIndex);
        } else {
          host = afterProtocol;
          path = '/';
        }
      }
    } else if (url.includes('/')) {
      const slashIndex = url.indexOf('/');
      host = url.substring(0, slashIndex);
      path = url.substring(slashIndex);
    }
  }
  
  // Ensure path starts with /
  if (!path.startsWith('/')) {
    path = '/' + path;
  }
  
  // Start with request line using only the path
  let request = `${method} ${path} HTTP/1.1\r\n`;
  
  // Add Host header if not already present
  if (host && !headers['Host'] && !headers['host']) {
    request += `Host: ${host}\r\n`;
  }
  
  // Add other headers
  for (const [key, value] of Object.entries(headers)) {
    request += `${key}: ${value}\r\n`;
  }
  
  // Add body if present
  if (body) {
    request += `\r\n${body}`;
  } else {
    request += `\r\n`;
  }
  
  return request;
}

/**
 * Apply syntax highlighting classes to an HTTP request string
 */
export function highlightHttpSyntax(httpRequest: string): string {
  if (!httpRequest) return '';
  
  // Split the request into lines
  const lines = httpRequest.split('\r\n');
  
  // Process the request line (first line)
  let result = '';
  if (lines.length > 0) {
    const requestLine = lines[0];
    // Highlight method, path, and HTTP version
    const parts = requestLine.match(/^([A-Z]+)\s+([^\s]+)\s+(HTTP\/[0-9.]+)$/);
    
    if (parts && parts.length === 4) {
      result += `<span class="http-method">${parts[1]}</span> `;
      result += `<span class="http-path">${parts[2]}</span> `;
      result += `<span class="http-version">${parts[3]}</span>\r\n`;
    } else {
      // Fall back to basic syntax highlighting if the pattern doesn't match
      result += highlightRequestLine(requestLine) + '\r\n';
    }
  }
  
  // Process the headers
  let inHeaders = true;
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i];
    
    // Empty line marks the end of headers
    if (line === '') {
      inHeaders = false;
      result += '\r\n';
      continue;
    }
    
    if (inHeaders) {
      // Highlight header key and value
      const headerParts = line.split(':');
      if (headerParts.length >= 2) {
        const key = headerParts[0];
        const value = headerParts.slice(1).join(':');
        result += `<span class="http-header-key">${key}</span>: <span class="http-header-value">${value.trim()}</span>\r\n`;
      } else {
        // Fall back for malformed headers
        result += line + '\r\n';
      }
    } else {
      // Body content
      result += `<span class="http-body">${line}</span>\r\n`;
    }
  }
  
  return result;
}

/**
 * Highlight the request line (method, path, HTTP version)
 */
function highlightRequestLine(line: string): string {
  // Match the HTTP method at the start of the line
  const methodMatch = line.match(/^([A-Z]+)/);
  if (!methodMatch) return line;
  
  const method = methodMatch[1];
  const rest = line.substring(method.length);
  
  // Split the rest into path and HTTP version
  const parts = rest.match(/^\s+([^\s]+)\s+(HTTP\/[0-9.]+)$/);
  if (!parts || parts.length < 3) {
    return `<span class="http-method">${method}</span>${rest}`;
  }
  
  const path = parts[1];
  const version = parts[2];
  
  return `<span class="http-method">${method}</span> <span class="http-path">${path}</span> <span class="http-version">${version}</span>`;
}

/**
 * Parse a raw HTTP request into its components
 */
export function parseRawHttpRequest(rawRequest: string): { method: string, url: string, headers: Record<string, string>, body: string } {
  const lines = rawRequest.split('\r\n');
  
  // Parse request line
  const requestLine = lines[0] || '';
  const requestLineParts = requestLine.match(/^([A-Z]+)\s+([^\s]+)\s+(HTTP\/[0-9.]+)$/);
  
  let method = 'GET';
  let path = '/';
  
  if (requestLineParts && requestLineParts.length >= 3) {
    method = requestLineParts[1];
    path = requestLineParts[2];
  }
  
  // Parse headers
  const headers: Record<string, string> = {};
  let headerEndIndex = -1;
  let host = '';
  
  for (let i = 1; i < lines.length; i++) {
    if (lines[i] === '') {
      headerEndIndex = i;
      break;
    }
    
    const headerLine = lines[i];
    const colonIndex = headerLine.indexOf(':');
    if (colonIndex > 0) {
      const key = headerLine.substring(0, colonIndex).trim();
      const value = headerLine.substring(colonIndex + 1).trim();
      headers[key] = value;
      
      // Extract host from Host header
      if (key.toLowerCase() === 'host') {
        host = value;
      }
    }
  }
  
  // Build full URL from path and host
  let url = path;
  if (host && !path.includes('://')) {
    // If path is already an absolute URL, use it as is
    if (!path.startsWith('http://') && !path.startsWith('https://')) {
      url = 'http://' + host + (path.startsWith('/') ? path : '/' + path);
    }
  }
  
  // Parse body
  let body = '';
  if (headerEndIndex !== -1 && headerEndIndex < lines.length - 1) {
    body = lines.slice(headerEndIndex + 1).join('\r\n');
  }
  
  return { method, url, headers, body };
}

 