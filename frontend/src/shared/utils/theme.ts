// Icy Midnight theme definition
export const theme = {
  name: "Icy Midnight",
  colors: {
    // Base theme colors
    midnight: "#1a1d24",
    midnightLight: "#242830",
    midnightDarker: "#16191f",
    midnightAccent: "#62dafc",
    secondaryAccent: "#62dafc",
    buttonText: "#111317",
    
    // Simplified navigation colors
    navText: "#e8e8e8",
    navTextHover: "#ffffff",
    navTextActive: "#62dafc",
    navBgActive: "#152636",
    navBorderActive: "#62dafc",
    
    // Form controls
    searchBarBg: "#2a303c",
    searchBarText: "#d1d5db",

    // Table styling
    tableBorder: "#2c3142",
    tableRowEven: "#1e222a",
    tableRowOdd: "#252a35",
    tableHeaderSeparator: "#323750",
    tableRowHover: "#2b3140",
    
    // Logo colors
    logoGradientStart: "#62dafc",
    logoGradientMid: "#40c4e7",
    logoGradientEnd: "#62dafc",
    logoShimmerBase: "rgba(98, 218, 252, 0.02)",
    logoShimmerMid: "rgba(98, 218, 252, 0.1)",
    logoShimmerHigh: "rgba(98, 218, 252, 0.2)",
    
    // UI grayscale
    gray100: "#f3f4f6",
    gray200: "#e5e7eb",
    gray300: "#d1d5db",
    gray400: "#9ca3af",
    gray500: "#6b7280",
    gray600: "#4b5563",
    gray700: "#374151",
    gray800: "#1f2937",
    
    // Status colors
    success: "#10b981",
    warning: "#f59e0b",
    danger: "#ef4444",
    info: "#3b82f6",
    
    // Method colors for HTTP requests
    methodGet: "#60a5fa",
    methodPost: "#4ade80",
    methodPut: "#facc15",
    methodDelete: "#f87171",
    methodPatch: "#c084fc",
    methodOther: "#9ca3af",
    
    // Status code colors
    status1xx: "#9ca3af",
    status2xx: "#10b981",
    status3xx: "#8b5cf6",
    status4xx: "#f59e0b",
    status5xx: "#ef4444",
    statusOther: "#ec4899",
  },
  gradients: {
    logo: "linear-gradient(135deg, #62dafc 0%, #40c4e7 50%, #62dafc 100%)",
    buttonHover: "linear-gradient(0deg, #a0e9ff 0%, #62dafc 100%)",
    navActiveGlow: "linear-gradient(0deg, rgba(42, 82, 152, 0.2) 0%, rgba(98, 218, 252, 0.1) 100%)",
  },
  animations: {
    shimmerDuration: "7s",
  },
  typography: {
    buttonWeight: "600",
  }
};

// Add CSS variables to the document root
export function initTheme() {
  const root = document.documentElement;
  
  // Set main theme colors
  root.style.setProperty('--color-midnight', theme.colors.midnight);
  root.style.setProperty('--color-midnight-light', theme.colors.midnightLight);
  root.style.setProperty('--color-midnight-darker', theme.colors.midnightDarker);
  root.style.setProperty('--color-midnight-accent', theme.colors.midnightAccent);
  root.style.setProperty('--color-secondary-accent', theme.colors.secondaryAccent);
  root.style.setProperty('--color-button-text', theme.colors.buttonText);
  
  // Set simplified navigation colors
  root.style.setProperty('--color-nav-text', theme.colors.navText);
  root.style.setProperty('--color-nav-text-hover', theme.colors.navTextHover);
  root.style.setProperty('--color-nav-text-active', theme.colors.navTextActive);
  root.style.setProperty('--color-nav-bg-active', theme.colors.navBgActive);
  root.style.setProperty('--color-nav-border-active', theme.colors.navBorderActive);
  
  // Set form controls colors
  root.style.setProperty('--color-search-bar-bg', theme.colors.searchBarBg);
  root.style.setProperty('--color-search-bar-text', theme.colors.searchBarText);
  
  // Set table styling colors
  root.style.setProperty('--color-table-border', theme.colors.tableBorder);
  root.style.setProperty('--color-table-row-even', theme.colors.tableRowEven);
  root.style.setProperty('--color-table-row-odd', theme.colors.tableRowOdd);
  root.style.setProperty('--color-table-header-separator', theme.colors.tableHeaderSeparator);
  root.style.setProperty('--color-table-row-hover', theme.colors.tableRowHover);
  
  // Set logo colors
  root.style.setProperty('--color-logo-gradient-start', theme.colors.logoGradientStart);
  root.style.setProperty('--color-logo-gradient-mid', theme.colors.logoGradientMid);
  root.style.setProperty('--color-logo-gradient-end', theme.colors.logoGradientEnd);
  root.style.setProperty('--color-logo-shimmer-base', theme.colors.logoShimmerBase);
  root.style.setProperty('--color-logo-shimmer-mid', theme.colors.logoShimmerMid);
  root.style.setProperty('--color-logo-shimmer-high', theme.colors.logoShimmerHigh);
  
  // Set UI colors
  root.style.setProperty('--color-gray-100', theme.colors.gray100);
  root.style.setProperty('--color-gray-200', theme.colors.gray200);
  root.style.setProperty('--color-gray-300', theme.colors.gray300);
  root.style.setProperty('--color-gray-400', theme.colors.gray400);
  root.style.setProperty('--color-gray-500', theme.colors.gray500);
  root.style.setProperty('--color-gray-600', theme.colors.gray600);
  root.style.setProperty('--color-gray-700', theme.colors.gray700);
  root.style.setProperty('--color-gray-800', theme.colors.gray800);
  
  // Set status colors
  root.style.setProperty('--color-success', theme.colors.success);
  root.style.setProperty('--color-warning', theme.colors.warning);
  root.style.setProperty('--color-danger', theme.colors.danger);
  root.style.setProperty('--color-info', theme.colors.info);
  
  // Set method colors
  root.style.setProperty('--color-method-get', theme.colors.methodGet);
  root.style.setProperty('--color-method-post', theme.colors.methodPost);
  root.style.setProperty('--color-method-put', theme.colors.methodPut);
  root.style.setProperty('--color-method-delete', theme.colors.methodDelete);
  root.style.setProperty('--color-method-patch', theme.colors.methodPatch);
  root.style.setProperty('--color-method-other', theme.colors.methodOther);
  
  // Set status code colors
  root.style.setProperty('--color-status-1xx', theme.colors.status1xx);
  root.style.setProperty('--color-status-2xx', theme.colors.status2xx);
  root.style.setProperty('--color-status-3xx', theme.colors.status3xx);
  root.style.setProperty('--color-status-4xx', theme.colors.status4xx);
  root.style.setProperty('--color-status-5xx', theme.colors.status5xx);
  root.style.setProperty('--color-status-other', theme.colors.statusOther);
  
  // Set gradients
  root.style.setProperty('--gradient-logo', theme.gradients.logo);
  root.style.setProperty('--gradient-button-hover', theme.gradients.buttonHover);
  root.style.setProperty('--gradient-nav-active-glow', theme.gradients.navActiveGlow);
  
  // Set animations
  root.style.setProperty('--animation-shimmer-duration', theme.animations.shimmerDuration);
} 

export const hexToRgba = (hex: string, alpha: number = 1): string => {
  if (typeof hex !== 'string' || !hex.startsWith('#')) {
    return 'rgba(0,0,0,0)'; 
  }

  let hexValue = hex.slice(1); 

  if (hexValue.length === 3) {
    hexValue = hexValue.split('').map(char => char + char).join('');
  }

  if (hexValue.length !== 6) {
    return 'rgba(0,0,0,0)';
  }

  const r = parseInt(hexValue.substring(0, 2), 16);
  const g = parseInt(hexValue.substring(2, 4), 16);
  const b = parseInt(hexValue.substring(4, 6), 16);

  if (isNaN(r) || isNaN(g) || isNaN(b)) {
    return 'rgba(0,0,0,0)';
  }

  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
};

