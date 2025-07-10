<script lang="ts">
    import { onMount, onDestroy } from 'svelte';
    import { File, FolderOpen, Plus, X, ChevronDown, ChevronRight, ShieldCheck, ShieldOff, ChevronUp } from 'lucide-svelte';
    import { ImportAPICollection, ImportAPICollectionAsync, GetAPICollections, BrowseForAPICollectionFile, SetAPICollectionSecurity, GetRequestWithSecurity, GetRequestExamplesWithSecurity, SetSelectedAPICollection, GetSelectedAPICollection, CopyRequestToClipboard, CopyAPIRequestToClipboard, CopyAPIRequestToCurrentFlow, DeleteAPICollection } from '../../../wailsjs/go/backend/App';
    import MonacoEditor from '../../components/monaco/MonacoEditor.svelte';
    import * as wailsRuntime from '../../../wailsjs/runtime/runtime';
    import ContextMenu from '../../shared/components/ContextMenu.svelte';
    import Notification from '../../shared/components/Notification.svelte';

    // Basic type for API collection
    type APICollection = {
        id: string;
        name: string;
        description: string;
        version: string;
        variables: APIVariable[];
        requests: APIRequest[];
        filePath: string;
        type: string;
        securitySchemes: APISecurityScheme[];
        activeSecurity: string;
    };

    type APIVariable = {
        name: string;
        value: string;
        description: string;
    };

    type APIRequest = {
        id: string;
        name: string;
        description: string;
        method: string;
        url: string;
        path: string;
        headers: APIHeader[];
        body: string;
        examples: APIExample[];
        parameters: APIParameter[];
        folder?: string;
    };

    type APIHeader = {
        name: string;
        value: string;
    };

    type APIExample = {
        id: string;
        name: string;
        request: string;
        response: string;
    };

    type APIParameter = {
        name: string;
        in: string;
        required: boolean;
        type: string;
        description: string;
    };

    type APISecurityScheme = {
        id: string;
        name: string;
        type: string;
        description: string;
        in?: string;
        scheme?: string;
        bearerFormat?: string;
        keyName?: string;
        value?: string;
    };

    type CollectionTab = {
        id: string;
        name: string;
        collection: APICollection | null;
    };

    // Import event types
    interface ImportSuccessEvent {
        importID: string;
        collection: APICollection;
    }

    interface ImportErrorEvent {
        importID: string;
        error: string;
    }

    // State
    let collections: APICollection[] = [];
    let tabs: CollectionTab[] = [{ id: 'default', name: 'Collection 1', collection: null }];
    let activeTabId = 'default';
    let isImporting = false;
    let selectedRequestId: string | null = null;
    let expandedFolders = new Set<string>();
    let expandedRequests = new Set<string>();
    let requestContent = '';
    let responseContent = '';
    let selectedExample: APIExample | null = null;
    let isDescriptionCollapsed = true;
    let isSecurityExpanded = false;
    let activeSecurityScheme: APISecurityScheme | null = null;
    let securityValue = '';
    
    // Add a key to force Monaco Editor re-render when content changes
    let monacoRequestKey = 0;
    let monacoResponseKey = 0;
    
    // Context menu state
    let showContextMenu = false;
    let contextMenuX = 0;
    let contextMenuY = 0;
    let contextMenuRequestId: string | null = null;
    let contextMenuExampleId: string | null = null;
    let showCopiedNotification = false;
    let notificationMessage = '';
    let contextMenu: ContextMenu;
    let notification: Notification;
    
    // Add request object reference for context menu
    let contextMenuRequest: APIRequest | null = null;
    
    // No longer needed - using selected flow directly
    
    // Debounce mechanism for request updates
    let updateTimeout: NodeJS.Timeout | null = null;

    // Derived values
    $: activeCollection = tabs.find(tab => tab.id === activeTabId)?.collection || null;
    $: groupedRequests = activeCollection ? groupRequestsByFolder(activeCollection.requests) : {};
    $: folderNames = Object.keys(groupedRequests).sort();
    $: hasCollections = collections.length > 0;
    $: {
        if (activeCollection) {
            // Find active security scheme
            if (activeCollection.securitySchemes && activeCollection.securitySchemes.length > 0) {
                const scheme = activeCollection.securitySchemes.find(s => s.id === activeCollection.activeSecurity);
                activeSecurityScheme = scheme || null;
                if (activeSecurityScheme) {
                    securityValue = activeSecurityScheme.value || '';
                } else {
                    securityValue = '';
                }
            } else {
                activeSecurityScheme = null;
                securityValue = '';
            }
        } else {
            activeSecurityScheme = null;
            securityValue = '';
        }
    }

    // Load collections on mount
    onMount(async () => {
        try {
            collections = await GetAPICollections() || [];
            
            // No longer needed - using selected flow directly
            
            // If collections exist, create tabs for ALL collections
            if (collections.length > 0) {
                console.log(`Loading ${collections.length} collections on mount`);
                
                // Get the selected collection ID
                const selectedCollectionID = await GetSelectedAPICollection();
                
                // Create tabs for all collections
                const newTabs: CollectionTab[] = [];
                let activeTabIndex = 0;
                
                for (let i = 0; i < collections.length; i++) {
                    const collection = collections[i];
                    const tab: CollectionTab = {
                        id: collection.id,
                        name: collection.name || `Collection ${i + 1}`,
                        collection: collection
                    };
                    
                    // If this is the selected collection, remember its index
                    if (selectedCollectionID && collection.id === selectedCollectionID) {
                        activeTabIndex = i;
                    }
                    
                    newTabs.push(tab);
                    
                    // Set up security for each collection
                    if (collection.securitySchemes && collection.securitySchemes.length > 0) {
                        // If no active security is set, set the first one
                        if (!collection.activeSecurity) {
                            const firstScheme = collection.securitySchemes[0];
                            console.log(`Auto-selecting first security scheme for ${collection.name}:`, firstScheme.name);
                            
                            // Update local state
                            collection.activeSecurity = firstScheme.id;
                            
                            // Save to backend
                            try {
                                await SetAPICollectionSecurity(collection.id, firstScheme.id, firstScheme.value || '');
                                console.log('Successfully set initial security scheme');
                            } catch (error) {
                                console.error('Failed to set initial security scheme:', error);
                            }
                        }
                    }
                }
                
                // Replace the default tab with all collection tabs
                tabs = newTabs;
                
                // Set the active tab (either selected collection or first one)
                if (tabs.length > activeTabIndex) {
                    activeTabId = tabs[activeTabIndex].id;
                    
                    // Set up security state for the active collection
                    const activeCollection = tabs[activeTabIndex].collection;
                    if (activeCollection && activeCollection.securitySchemes && activeCollection.securitySchemes.length > 0) {
                        const scheme = activeCollection.securitySchemes.find(s => s.id === activeCollection.activeSecurity);
                        if (scheme) {
                            activeSecurityScheme = scheme;
                            securityValue = scheme.value || '';
                        }
                    }
                }
                
                console.log(`Created ${tabs.length} tabs, active tab: ${activeTabId}`);
            }
            
            // Set up event listeners for import success/error
            wailsRuntime.EventsOn("api-collection:import-success", (data) => {
                isImporting = false;
                const collection = data.collection;
                collections = [...collections, collection];
                
                // Add to tabs or use the first tab if empty
                if (!tabs[0].collection) {
                    tabs[0].collection = collection;
                    tabs[0].name = collection.name || 'Collection 1';
                    tabs[0].id = collection.id; // Fix: Update tab ID to match collection ID
                    activeTabId = collection.id; // Fix: Update active tab ID too
                    
                    // Auto-select the first security scheme if available
                    if (collection.securitySchemes && collection.securitySchemes.length > 0) {
                        const firstScheme = collection.securitySchemes[0];
                        collection.activeSecurity = firstScheme.id;
                        activeSecurityScheme = firstScheme;
                        securityValue = firstScheme.value || '';
                        
                        // Save this selection to the backend
                        try {
                            SetAPICollectionSecurity(collection.id, firstScheme.id, firstScheme.value || '');
                            console.log('Successfully set security scheme for new import');
                        } catch (error) {
                            console.error('Failed to set initial security scheme:', error);
                        }
                    }
                } else {
                    const newTab = {
                        id: collection.id,
                        name: collection.name || 'Collection',
                        collection
                    };
                    
                    // Auto-select the first security scheme if available
                    if (collection.securitySchemes && collection.securitySchemes.length > 0) {
                        const firstScheme = collection.securitySchemes[0];
                        collection.activeSecurity = firstScheme.id;
                        
                        // Save this selection to the backend
                        try {
                            SetAPICollectionSecurity(collection.id, firstScheme.id, firstScheme.value || '');
                            console.log('Successfully set security scheme for new import in new tab');
                        } catch (error) {
                            console.error('Failed to set initial security scheme:', error);
                        }
                    }
                    
                    tabs = [...tabs, newTab];
                    activeTabId = collection.id;
                }
                
                // For development purposes, log collection details
                console.log('Import successful:', collection.name);
            });
            
            wailsRuntime.EventsOn("api-collection:import-error", (data) => {
                isImporting = false;
                console.error('Import failed:', data.error);
                alert(`Import failed: ${data.error}`);
            });
            
            // Set up event listener for security changes - enables real-time preview updates
            console.log('🔧 Setting up api-collection:security-changed event listener');
            wailsRuntime.EventsOn("api-collection:security-changed", (data) => {
                console.log('🔧 🎉 Security changed event received for real-time update:', data);
                const { collectionID, collection: updatedCollection } = data;
                
                // Update the collection in tabs
                for (let i = 0; i < tabs.length; i++) {
                    const currentTab = tabs[i];
                    if (currentTab.collection && currentTab.collection.id === collectionID) {
                        tabs[i].collection = updatedCollection;
                        
                        // If this is the active tab, update the reactive state
                        if (currentTab.id === activeTabId) {
                            // Find the active security scheme
                            if (updatedCollection && updatedCollection.securitySchemes && updatedCollection.securitySchemes.length > 0) {
                                const scheme = updatedCollection.securitySchemes.find((s: APISecurityScheme) => s.id === updatedCollection.activeSecurity);
                                if (scheme) {
                                    activeSecurityScheme = scheme;
                                    securityValue = scheme.value || '';
                                } else {
                                    activeSecurityScheme = null;
                                    securityValue = '';
                                }
                            } else {
                                activeSecurityScheme = null;
                                securityValue = '';
                            }
                            
                            // REAL-TIME PREVIEW UPDATE: Refresh the currently selected request
                            if (selectedRequestId) {
                                console.log('🔄 REAL-TIME UPDATE: Updating selected request preview due to security change');
                                console.log('🔄 Selected request ID:', selectedRequestId);
                                console.log('🔄 Active security scheme:', activeSecurityScheme?.name || 'None');
                                console.log('🔄 Security value:', securityValue);
                                
                                // Force update the request preview
                                updateSelectedRequest().then(() => {
                                    console.log('🔄 Request preview updated successfully');
                                }).catch(error => {
                                    console.error('🔄 Failed to update request preview:', error);
                                });
                                
                                // Also refresh the selected example if one is active
                                if (selectedExample && activeRequest) {
                                    console.log('🔄 Refreshing selected example with new security');
                                    selectExample(activeRequest, selectedExample);
                                }
                            } else {
                                console.log('🔄 No request selected, skipping preview update');
                            }
                            
                            // Refresh all expanded request examples
                            for (const expandedId of expandedRequests) {
                                // Find the request and refresh its examples
                                for (const folderName of folderNames) {
                                    const requests = groupedRequests[folderName] || [];
                                    for (const request of requests) {
                                        if (request.id === expandedId) {
                                            GetRequestExamplesWithSecurity(updatedCollection.id, expandedId)
                                                .then(examples => {
                                                    if (examples && examples.length > 0) {
                                                        request.examples = examples;
                                                        console.log(`Refreshed examples for request ${expandedId} with new security`);
                                                    }
                                                })
                                                .catch(error => {
                                                    console.error('Failed to refresh examples with security:', error);
                                                });
                                            break;
                                        }
                                    }
                                }
                            }
                        }
                        break;
                    }
                }
                
                // Update collections array
                collections = collections.map((c: APICollection) => 
                    c.id === collectionID ? updatedCollection : c
                );
                
                console.log('Real-time security change applied to frontend with preview update');
            });
            
        } catch (error) {
            console.error('Failed to load collections:', error);
        }
    });
    
    // Clean up event listeners on unmount
    onDestroy(() => {
        wailsRuntime.EventsOff("api-collection:import-success");
        wailsRuntime.EventsOff("api-collection:import-error");
        wailsRuntime.EventsOff("api-collection:security-changed");
        
        // Clean up any pending timeout
        if (updateTimeout) {
            clearTimeout(updateTimeout);
        }
    });
    
    // Group requests by folder for display
    function groupRequestsByFolder(requests: APIRequest[]): Record<string, APIRequest[]> {
        const grouped: Record<string, APIRequest[]> = { "": [] };
        
        if (!requests) return grouped;
        
        requests.forEach(request => {
            const folder = request.folder || "";
            if (!grouped[folder]) {
                grouped[folder] = [];
            }
            grouped[folder].push(request);
        });
        
        return grouped;
    }
    
    // Get color for HTTP method
    function getMethodColor(method: string): string {
        const methodLower = method.toLowerCase();
        switch (methodLower) {
            case 'get': return 'method-get';
            case 'post': return 'method-post';
            case 'put': return 'method-put';
            case 'delete': return 'method-delete';
            case 'patch': return 'method-patch';
            case 'head': return 'method-head';
            case 'options': return 'method-options';
            default: return 'text-gray-50';
        }
    }
    
    // Import collection
    async function importCollection() {
        try {
            isImporting = true;
            
            // Use the dedicated file browser function
            let filePath;
            try {
                filePath = await BrowseForAPICollectionFile();
            } catch (error) {
                console.error('Failed to browse for file:', error);
                isImporting = false;
                return;
            }
            
            // Now use the async import function which will return immediately
            // and notify us through events when done
            try {
                await ImportAPICollectionAsync(filePath);
                console.log('Import started in background, waiting for completion...');
                // The UI will update based on the events we're listening to
                // isImporting will be set to false in the event handlers
            } catch (error) {
                console.error('Failed to start import:', error);
                const errorMsg = error instanceof Error ? error.message : 'Unknown error';
                alert(`Failed to start import: ${errorMsg}`);
                isImporting = false;
            }
        } catch (error) {
            isImporting = false;
            console.error('Import process failed:', error);
            alert('Import failed. See console for details.');
        }
    }
    
    // Add new tab
    function addTab() {
        const newTabId = `tab-${Date.now()}`;
        tabs = [...tabs, { id: newTabId, name: `Collection ${tabs.length + 1}`, collection: null }];
        activeTabId = newTabId;
    }
    
    // Close tab
    async function closeTab(id: string) {
        // Don't close if it's the only tab
        if (tabs.length <= 1) return;
        
        // Find the tab to get the collection
        const tab = tabs.find(tab => tab.id === id);
        if (tab && tab.collection) {
            try {
                // Delete the collection from the backend
                await DeleteAPICollection(tab.collection.id);
                console.log(`Deleted collection: ${tab.collection.name}`);
                
                // Update the collections array
                collections = collections.filter(c => c.id !== tab.collection!.id);
            } catch (error) {
                console.error('Failed to delete collection:', error);
                // Still remove the tab from UI even if backend deletion failed
            }
        }
        
        // Remove the tab
        tabs = tabs.filter(tab => tab.id !== id);
        
        // If we closed the active tab, activate the first tab
        if (id === activeTabId) {
            activeTabId = tabs[0].id;
        }
    }
    
    // Set active tab
    function setActiveTab(id: string) {
        activeTabId = id;
        
        // Find the collection for this tab
        const tab = tabs.find(tab => tab.id === id);
        if (tab && tab.collection) {
            // Set as selected collection for the project
            // This will ensure it's saved with the project
            SetSelectedAPICollection(tab.collection.id);
        }
    }
    
    // Toggle folder expansion
    function toggleFolder(folder: string) {
        if (expandedFolders.has(folder)) {
            expandedFolders.delete(folder);
        } else {
            expandedFolders.add(folder);
        }
        expandedFolders = expandedFolders; // Trigger reactivity
    }
    
    // Toggle request expansion
    async function toggleRequest(id: string) {
        if (expandedRequests.has(id)) {
            expandedRequests.delete(id);
        } else {
            expandedRequests.add(id);
            
            // When expanding a request, also select it
            if (!selectedRequestId || selectedRequestId !== id) {
                selectedRequestId = id;
                await updateSelectedRequest();
            }
            
            // When expanding a request, fetch examples with security if there's an active scheme
            if (activeCollection && activeCollection.securitySchemes && activeCollection.securitySchemes.length > 0 && activeSecurityScheme && activeSecurityScheme.value) {
                try {
                    // Find the request
                    for (const folderName of folderNames) {
                        const requests = groupedRequests[folderName] || [];
                        for (const request of requests) {
                            if (request.id === id) {
                                // Get examples with security applied
                                const examples = await GetRequestExamplesWithSecurity(activeCollection.id, id);
                                if (examples && examples.length > 0) {
                                    // Update the examples in the request
                                    request.examples = examples;
                                }
                                break;
                            }
                        }
                    }
                } catch (error) {
                    console.error('Failed to get request examples with security:', error);
                }
            }
        }
        expandedRequests = expandedRequests; // Trigger reactivity
    }
    
    // Toggle description
    function toggleDescription() {
        isDescriptionCollapsed = !isDescriptionCollapsed;
    }
    
    // Toggle security panel
    function toggleSecurity() {
        isSecurityExpanded = !isSecurityExpanded;
    }
    
    // Set active security scheme
    async function setActiveSecurity(scheme: APISecurityScheme) {
        if (!activeCollection) return;
        
        try {
            // Update local state first for immediate feedback
            activeCollection.activeSecurity = scheme.id;
            activeSecurityScheme = scheme;
            securityValue = scheme.value || '';
            
            // Then try to save to backend
            await SetAPICollectionSecurity(activeCollection.id, scheme.id, securityValue);
            
            // If a request is selected, reload it with the new security
            if (selectedRequestId) {
                await updateSelectedRequest();
                
                // If any requests are expanded, refresh their examples too
                for (const expandedId of expandedRequests) {
                    // Find the request
                    for (const folderName of folderNames) {
                        const requests = groupedRequests[folderName] || [];
                        for (const request of requests) {
                            if (request.id === expandedId) {
                                try {
                                    // Get examples with security applied
                                    const examples = await GetRequestExamplesWithSecurity(activeCollection.id, expandedId);
                                    if (examples && examples.length > 0) {
                                        // Update the examples in the request
                                        request.examples = examples;
                                    }
                                } catch (error) {
                                    console.error('Failed to refresh examples with security:', error);
                                }
                                break;
                            }
                        }
                    }
                }
            }
        } catch (error) {
            console.error('Failed to update security scheme:', error);
            // Show error notification here if desired
        }
    }
    
    // Update security value when user finishes editing
    function updateSecurityValue() {
        if (!activeCollection || !activeSecurityScheme) return;
        
        // Update the local state
        const schemeIndex = activeCollection.securitySchemes.findIndex(s => s.id === activeSecurityScheme!.id);
        if (schemeIndex >= 0) {
            activeCollection.securitySchemes[schemeIndex].value = securityValue;
            activeSecurityScheme!.value = securityValue;
        }
        
        // Save to backend
        SetAPICollectionSecurity(activeCollection.id, activeSecurityScheme.id, securityValue)
            .then(() => {
                // If a request is selected, reload it with the new security
                if (selectedRequestId) {
                    updateSelectedRequest();
                }
            })
            .catch(error => {
                console.error('Failed to update security value:', error);
            });
    }
    

    
    // Select request with debouncing
    function selectRequest(request: APIRequest) {
        selectedRequestId = request.id;
        selectedExample = null;
        responseContent = ''; // Clear the response content when a new request is selected
        
        // Clear any pending update
        if (updateTimeout) {
            clearTimeout(updateTimeout);
        }
        
        // Debounce the update to prevent rapid successive calls
        updateTimeout = setTimeout(async () => {
            await updateSelectedRequest();
            updateTimeout = null;
        }, 100);
    }
    
    // Select example with debouncing
    function selectExample(request: APIRequest, example: APIExample) {
        selectedExample = example;
        
        // Clear any pending update
        if (updateTimeout) {
            clearTimeout(updateTimeout);
        }
        
        // Debounce the update to prevent rapid successive calls
        updateTimeout = setTimeout(async () => {
            try {
                if (!activeCollection) return;
                
                // Get the example with security applied
                const examples = await GetRequestExamplesWithSecurity(
                    activeCollection.id,
                    request.id
                );
                
                // Find the matching example in the returned array
                const securedExample = examples.find(ex => ex.id === example.id);
                
                if (securedExample) {
                    requestContent = securedExample.request || '';
                    responseContent = securedExample.response || '';
                } else {
                    // Fallback to the original example if not found
                    requestContent = example.request || '';
                    responseContent = example.response || '';
                }
                
                // Force Monaco Editors to refresh
                monacoRequestKey++;
                monacoResponseKey++;
                console.log('🔄 🔑 Forcing Monaco Editors refresh for example, keys:', monacoRequestKey, monacoResponseKey);
            } catch (error) {
                console.error('Failed to get request example with security:', error);
                requestContent = example.request || '';
                responseContent = example.response || '';
            } finally {
                updateTimeout = null;
            }
        }, 100);
    }
    
    // Update selected request with security
    async function updateSelectedRequest() {
        if (!activeCollection || !selectedRequestId) {
            console.log('🔄 updateSelectedRequest: Missing activeCollection or selectedRequestId');
            return;
        }
        
        console.log('🔄 updateSelectedRequest called');
        console.log('🔄 Active collection:', activeCollection.name);
        console.log('🔄 Selected request ID:', selectedRequestId);
        console.log('🔄 Active security scheme:', activeSecurityScheme?.name || 'None');
        console.log('🔄 Security value:', securityValue);
        
        const oldRequestContent = requestContent;
        
        try {
            // Always apply security if a scheme is selected, even if no value is set
            if (activeCollection.securitySchemes && activeCollection.securitySchemes.length > 0 && activeSecurityScheme) {
                console.log('🔄 Applying security to request:', activeSecurityScheme.name);
                // Get request with security applied
                const requestWithSecurity = await GetRequestWithSecurity(activeCollection.id, selectedRequestId);
                requestContent = requestWithSecurity;
                console.log('🔄 Updated request content with security applied');
                console.log('🔄 New request content:', requestContent.substring(0, 200) + '...');
            } else {
                console.log('🔄 No security scheme active, using default request format');
                // No security, use the default content
                const request = activeCollection.requests.find(r => r.id === selectedRequestId);
                if (request) {
                    requestContent = `${request.method} ${request.path}
${request.headers.map(h => `${h.name}: ${h.value}`).join('\n')}

${request.body || ''}`;
                    console.log('🔄 Updated request content without security');
                    console.log('🔄 New request content:', requestContent.substring(0, 200) + '...');
                }
            }
            
            // Check if content actually changed
            if (oldRequestContent !== requestContent) {
                console.log('🔄 ✅ Request content successfully updated!');
                // Force Monaco Editor to re-render by changing the key
                monacoRequestKey++;
                console.log('🔄 🔑 Forcing Monaco Editor refresh with key:', monacoRequestKey);
            } else {
                console.log('🔄 ⚠️ Request content did not change');
            }
            
        } catch (error) {
            console.error('🔄 ❌ Failed to get request with security:', error);
            
            // Fallback to basic display if there's an error
            const request = activeCollection.requests.find(r => r.id === selectedRequestId);
            if (request) {
                requestContent = `${request.method} ${request.path}
${request.headers.map(h => `${h.name}: ${h.value}`).join('\n')}

${request.body || ''}`;
                console.log('🔄 Used fallback request content');
            }
        }
    }

    // Handle request context menu
    function handleRequestContextMenu(event: MouseEvent, request: APIRequest) {
        event.preventDefault();
        event.stopPropagation();
        
        console.log('🔧 Right-click on request:', request.name);
        
        contextMenuRequestId = request.id;
        contextMenuExampleId = null;
        contextMenuX = event.clientX;
        contextMenuY = event.clientY;
        showContextMenu = true;
        contextMenuRequest = request;
    }
    
    // Handle example context menu
    function handleExampleContextMenu(event: MouseEvent, request: APIRequest, example: APIExample) {
        event.preventDefault();
        event.stopPropagation();
        
        console.log('🔧 Right-click on example:', example.name, 'from request:', request.name);
        
        contextMenuRequestId = request.id;
        contextMenuExampleId = example.id;
        contextMenuX = event.clientX;
        contextMenuY = event.clientY;
        showContextMenu = true;
        contextMenuRequest = request;
    }
    
    // Handle context menu close
    function handleContextMenuClose() {
        showContextMenu = false;
        contextMenuRequestId = null;
        contextMenuExampleId = null;
        contextMenuRequest = null;
    }
    
    // Copy request to clipboard
    async function copyRequestToClipboard() {
        if (!contextMenuRequest || !activeCollection) return;
        
        console.log('🔧 📋 Copying request to clipboard:', contextMenuRequest.name);
        
        try {
            // Call backend to copy API collection request to clipboard
            await CopyAPIRequestToClipboard(contextMenuRequest.id);
            
            // Show notification
            notificationMessage = "Request copied to clipboard";
            showCopiedNotification = true;
            setTimeout(() => {
                showCopiedNotification = false;
            }, 3000);
            
            console.log('🔧 ✅ Request copied successfully');
            
            // Close context menu
            handleContextMenuClose();
        } catch (error) {
            console.error('🔧 ❌ Failed to copy request:', error);
        }
    }
    
    // Copy request to selected flow
    async function copyRequestToSelectedFlow() {
        if (!contextMenuRequest || !activeCollection) return;
        
        console.log('🔧 🔄 Copying request to selected flow:', contextMenuRequest.name);
        
        try {
            // Call backend to copy request to the currently selected flow
            await CopyAPIRequestToCurrentFlow(activeCollection.id, contextMenuRequest.id);
            
            // Show notification
            notificationMessage = "Request added to selected flow";
            showCopiedNotification = true;
            setTimeout(() => {
                showCopiedNotification = false;
            }, 3000);
            
            console.log('🔧 ✅ Request added to selected flow successfully');
            
            // Close context menu
            handleContextMenuClose();
        } catch (error) {
            console.error('🔧 ❌ Failed to copy request to selected flow:', error);
        }
    }
    
    // Create context menu items
    function getContextMenuItems() {
        const items = [
            { label: 'Copy Request', onClick: copyRequestToClipboard },
            { label: 'Copy to Current Flow', onClick: copyRequestToSelectedFlow }
        ];
        
        return items;
    }

    // Handle security scheme selection from dropdown - triggers real-time preview updates
    function handleSecuritySchemeSelection(event: Event) {
        const target = event.target as HTMLSelectElement;
        const selectedSchemeId = target.value;
        
        if (!activeCollection) return;
        
        console.log('🔧 Security scheme selection changed:', selectedSchemeId);
        console.log('🔧 Current request selected:', selectedRequestId);
        console.log('🔧 Current request content preview length:', requestContent?.length || 0);
        
        if (!selectedSchemeId) {
            // "None" selected
            console.log('Setting security to None - will trigger real-time preview update');
            activeSecurityScheme = null;
            securityValue = '';
            activeCollection.activeSecurity = '';
        } else {
            // Find the selected scheme
            const scheme = activeCollection.securitySchemes.find(s => s.id === selectedSchemeId);
            if (scheme) {
                console.log('Setting security scheme to:', scheme.name, '- will trigger real-time preview update');
                activeSecurityScheme = scheme;
                
                // Set placeholder value based on scheme type
                if (scheme.type === 'apiKey') {
                    securityValue = 'your-api-key-here';
                } else if (scheme.type === 'http' && scheme.scheme === 'basic') {
                    securityValue = 'dXNlcm5hbWU6cGFzc3dvcmQ='; // username:password base64
                } else if (scheme.type === 'http' && scheme.scheme === 'bearer') {
                    securityValue = 'your-bearer-token-here';
                } else {
                    securityValue = 'your-auth-value-here';
                }
                
                activeCollection.activeSecurity = scheme.id;
            }
        }
        
        // IMMEDIATE TEST: Try updating the preview right away to test if Monaco responds
        if (selectedRequestId) {
            console.log('🧪 IMMEDIATE TEST: Updating preview right now to test responsiveness');
            updateSelectedRequest().then(() => {
                console.log('🧪 IMMEDIATE TEST: Preview update completed');
            }).catch(error => {
                console.error('🧪 IMMEDIATE TEST: Preview update failed:', error);
            });
        }
        
        // Save to backend (this will trigger the security-changed event for real-time preview updates)
        if (activeCollection && activeSecurityScheme) {
            console.log('🔧 Saving security scheme to backend - real-time preview will update via event');
            SetAPICollectionSecurity(activeCollection.id, activeSecurityScheme.id, securityValue)
                .then(() => {
                    console.log('🔧 Security scheme updated successfully - real-time preview update will happen via event');
                })
                .catch(error => {
                    console.error('🔧 Failed to set security scheme:', error);
                });
        } else {
            // Clear security
            console.log('🔧 Clearing security scheme in backend - real-time preview will update via event');
            SetAPICollectionSecurity(activeCollection.id, '', '')
                .then(() => {
                    console.log('🔧 Security cleared successfully - real-time preview update will happen via event');
                })
                .catch(error => {
                    console.error('🔧 Failed to clear security scheme:', error);
                });
        }
    }

    // Add computed property for activeRequest
    $: activeRequest = activeCollection && selectedRequestId 
        ? activeCollection.requests.find(r => r.id === selectedRequestId) 
        : null;

    // Fix MonacoEditor theme property issues
    // Remove the theme="vs-dark" attributes from both MonacoEditor components and set it in options

    // Add to selected flow function
    function addToSelectedFlow() {
        if (!activeCollection || !selectedRequestId) return;
        
        try {
            CopyAPIRequestToCurrentFlow(activeCollection.id, selectedRequestId)
                .then(() => {
                    // Show notification
                    notificationMessage = "Request added to selected flow";
                    showCopiedNotification = true;
                    setTimeout(() => {
                        showCopiedNotification = false;
                    }, 3000);
                })
                .catch((error: any) => {
                    console.error('Failed to copy request to selected flow:', error);
                });
        } catch (error) {
            console.error('Failed to copy request to selected flow:', error);
        }
    }

    // Fix usage of CopyRequestToClipboard
    // In the HTML:
    // on:click={() => selectedRequestId && CopyRequestToClipboard(selectedRequestId)}

    // Fix activeCollection.parameters references
    // Remove this block entirely:
    // {#if activeCollection.parameters && activeCollection.parameters.length > 0}
    // ...
    // {/if}
</script>

<div class="w-full h-full flex flex-col bg-[var(--color-midnight)] text-white">
    <!-- Tab bar with Import button -->
    <div class="flex bg-[var(--color-midnight-light)] px-2 border-b border-[var(--color-midnight-darker)]">
        <div class="flex overflow-x-auto flex-1 max-w-[calc(100%-180px)]">
            {#each tabs as tab}
                <button
                    class="px-3 py-2 flex items-center gap-2 cursor-pointer transition-colors border-b-2 min-w-[120px] max-w-[250px] {activeTabId === tab.id ? 'border-blue-500 bg-[var(--color-midnight-light)]/80' : 'border-transparent'}"
                    on:click={() => setActiveTab(tab.id)}
                    title={tab.name}
                >
                    <span class="truncate text-sm flex-1">{tab.name}</span>
                    {#if tabs.length > 1}
                        <div
                            class="text-gray-50 hover:text-white flex-shrink-0 ml-1 cursor-pointer"
                            role="button"
                            tabindex="0"
                            on:click={(e) => { e.stopPropagation(); closeTab(tab.id); }}
                            on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); closeTab(tab.id); } }}
                        >
                            <X size={12} />
                        </div>
                    {/if}
                </button>
            {/each}
        </div>
        <div class="flex items-center ml-auto px-2">
            <button class="px-4 py-0 bg-[var(--color-midnight-accent)] hover:bg-[var(--color-midnight-accent)]/80 text-[var(--color-button-text)] rounded-md flex items-center" on:click={importCollection}>
                <FolderOpen size={16} class="mr-2" />
                Import New Collection
            </button>
        </div>
    </div>
    
    <!-- Main content area with sidebar and content -->
    <div class="flex-1 flex overflow-hidden">
        <!-- Collection sidebar -->
        <div class="w-1/3 border-r border-[var(--color-midnight-darker)] overflow-auto p-2">
            {#if activeCollection}
                <div class="mb-2">
                    <div class="flex items-center justify-between mb-2">
                        <h2 class="text-lg font-semibold">{activeCollection.name}</h2>
                        <span class="text-xs bg-[var(--color-midnight-light)] px-2 py-1 rounded">
                            {activeCollection.type} {activeCollection.version}
                        </span>
                    </div>
                    
                    {#if activeCollection.description}
                        <div class="mb-4">
                            <div class="flex justify-between items-center mb-2">
                                <h3 class="text-sm font-medium text-gray-50">Description</h3>
                                <button 
                                    class="text-xs text-gray-50 hover:text-white"
                                    on:click={() => isDescriptionCollapsed = !isDescriptionCollapsed}
                                >
                                    {isDescriptionCollapsed ? 'Show' : 'Hide'}
                                </button>
                            </div>
                            {#if !isDescriptionCollapsed}
                                <div class="bg-[var(--color-midnight-light)] rounded-md p-2 text-sm text-gray-100 whitespace-pre-wrap">{activeCollection.description}</div>
                            {/if}
                        </div>
                    {/if}
                    


                    <!-- Security Configuration -->
                    {#if activeCollection.securitySchemes && activeCollection.securitySchemes.length > 0}
                        <div class="mb-4">
                            <div class="flex justify-between items-center mb-2">
                                <h3 class="text-sm font-medium text-gray-50">Security</h3>
                                <button 
                                    class="text-xs px-2 py-1 rounded-md flex items-center gap-1 transition-colors"
                                    class:bg-green-700={isSecurityExpanded && activeSecurityScheme && securityValue} 
                                    class:text-white={isSecurityExpanded && activeSecurityScheme && securityValue}
                                    class:bg-[var(--color-midnight-light)]={!isSecurityExpanded || !activeSecurityScheme || !securityValue}
                                    on:click={toggleSecurity}
                                >
                                    {#if isSecurityExpanded && activeSecurityScheme && securityValue}
                                        <ShieldCheck size={12} />
                                        Active
                                    {:else}
                                        <ShieldOff size={12} />
                                        Configure
                                    {/if}
                                </button>
                            </div>
                            
                            {#if isSecurityExpanded}
                                <div class="bg-[var(--color-midnight-light)] rounded-md p-2 space-y-2">
                                    <div>
                                        <h3 class="block text-xs font-medium mb-1">Security Scheme</h3>
                                        <select 
                                            class="w-full bg-[var(--color-midnight-darker)] rounded-md px-2 py-1 text-xs border border-[var(--color-midnight-darker)]"
                                            on:change={handleSecuritySchemeSelection}
                                        >
                                            <option value="">None</option>
                                            {#each activeCollection.securitySchemes as scheme}
                                                <option value={scheme.id} selected={activeSecurityScheme && activeSecurityScheme.id === scheme.id}>
                                                    {scheme.name} ({scheme.type})
                                                </option>
                                            {/each}
                                        </select>
                                    </div>
                                    
                                    {#if activeSecurityScheme}
                                        <div class="text-xs text-gray-100 bg-[var(--color-midnight-darker)] rounded-md p-2">
                                            <div class="font-medium mb-1">
                                                {#if activeSecurityScheme.type === 'apiKey'}
                                                    API Key Authentication
                                                {:else if activeSecurityScheme.type === 'http' && activeSecurityScheme.scheme === 'basic'}
                                                    Basic Authentication
                                                {:else if activeSecurityScheme.type === 'http' && activeSecurityScheme.scheme === 'bearer'}
                                                    Bearer Token Authentication
                                                {:else}
                                                    {activeSecurityScheme.name} Authentication
                                                {/if}
                                            </div>
                                            <div class="text-gray-50">
                                                Active with placeholder value: 
                                                <span class="font-mono text-[var(--color-midnight-accent)]">{securityValue}</span>
                                            </div>
                                        </div>
                                    {:else}
                                        <div class="text-xs text-gray-50 bg-[var(--color-midnight-darker)] rounded-md p-2">
                                            No authentication will be applied to requests
                                        </div>
                                    {/if}
                                </div>
                            {/if}
                        </div>
                    {/if}
                </div>
                
                <div>
                    <h3 class="text-sm font-medium text-gray-50 mb-1">Requests</h3>
                    <div class="bg-[var(--color-midnight-light)] rounded-md">
                        {#if activeCollection.requests && activeCollection.requests.length > 0}
                            <!-- Group by folder -->
                            {#each Array.from(new Set(activeCollection.requests.map(r => r.folder || ''))).sort() as folder}
                                {#if folder}
                                    <div class="border-b border-[var(--color-midnight-darker)] last:border-0">
                                        <button 
                                            class="flex items-center p-2 cursor-pointer hover:bg-[var(--color-midnight-light)]/80 w-full text-left"
                                            on:click={() => toggleFolder(folder)}
                                        >
                                            {#if expandedFolders.has(folder)}
                                                <ChevronDown size={16} class="mr-1" />
                                            {:else}
                                                <ChevronRight size={16} class="mr-1" />
                                            {/if}
                                            <span>{folder}</span>
                                        </button>
                                        
                                        {#if expandedFolders.has(folder)}
                                            <div class="pl-2">
                                                {#each activeCollection.requests.filter(r => r.folder === folder) as request}
                                                    <div class="border-t border-[var(--color-midnight-darker)]">
                                                        <!-- Request header -->
                                                        <div class="flex items-center p-2 hover:bg-[var(--color-midnight-light)]/80 {selectedRequestId === request.id ? 'bg-[var(--color-midnight-accent)]/20' : ''}">
                                                            {#if request.examples && request.examples.length > 0}
                                                                <button 
                                                                    class="mr-1 p-1 hover:bg-[var(--color-midnight-accent)]/20 rounded flex-shrink-0"
                                                                    on:click|stopPropagation={() => toggleRequest(request.id)}
                                                                >
                                                                    {#if expandedRequests.has(request.id)}
                                                                        <ChevronDown size={14} />
                                                                    {:else}
                                                                        <ChevronRight size={14} />
                                                                    {/if}
                                                                </button>
                                                            {/if}
                                                            <button 
                                                                class="flex items-center flex-1 cursor-pointer text-left min-w-0"
                                                                on:click={() => selectRequest(request)}
                                                                on:contextmenu|preventDefault={(e) => handleRequestContextMenu(e, request)}
                                                            >
                                                                <span class={`mr-2 px-1 text-xs rounded flex-shrink-0 ${getMethodColor(request.method)}`}>
                                                                    {request.method}
                                                                </span>
                                                                <div class="flex-1 min-w-0">
                                                                    <div class="font-medium truncate text-sm">{request.name || request.path}</div>
                                                                    <div class="text-xs text-gray-50 truncate">{request.path}</div>
                                                                </div>
                                                            </button>
                                                        </div>
                                                        
                                                        <!-- Examples (when request is expanded) -->
                                                        {#if expandedRequests.has(request.id) && request.examples && request.examples.length > 0}
                                                            <div class="pl-6 pb-2">
                                                                <div class="text-xs text-gray-50 mb-1">Examples:</div>
                                                                {#each request.examples as example}
                                                                    <button 
                                                                        class="block w-full text-left px-2 py-1 text-xs rounded-md mb-1 transition-colors {selectedExample && selectedExample.id === example.id ? 'bg-[var(--color-midnight-accent)]/40' : 'bg-[var(--color-midnight-light)] hover:bg-[var(--color-midnight-accent)]/20'}"
                                                                        on:click|stopPropagation={() => { selectRequest(request); selectExample(request, example); }}
                                                                        on:contextmenu|preventDefault={(e) => handleExampleContextMenu(e, request, example)}
                                                                    >
                                                                        {example.name}
                                                                    </button>
                                                                {/each}
                                                            </div>
                                                        {/if}
                                                    </div>
                                                {/each}
                                            </div>
                                        {/if}
                                    </div>
                                {:else}
                                    <!-- Requests without folder -->
                                    {#each activeCollection.requests.filter(r => !r.folder) as request}
                                        <div class="border-b border-[var(--color-midnight-darker)] last:border-0">
                                            <!-- Request header -->
                                            <div class="flex items-center p-2 hover:bg-[var(--color-midnight-light)]/80 {selectedRequestId === request.id ? 'bg-[var(--color-midnight-accent)]/20' : ''}">
                                                {#if request.examples && request.examples.length > 0}
                                                    <button 
                                                        class="mr-1 p-1 hover:bg-[var(--color-midnight-accent)]/20 rounded flex-shrink-0"
                                                        on:click|stopPropagation={() => toggleRequest(request.id)}
                                                    >
                                                        {#if expandedRequests.has(request.id)}
                                                            <ChevronDown size={14} />
                                                        {:else}
                                                            <ChevronRight size={14} />
                                                        {/if}
                                                    </button>
                                                {/if}
                                                <button 
                                                    class="flex items-center flex-1 cursor-pointer text-left min-w-0"
                                                    on:click={() => selectRequest(request)}
                                                    on:contextmenu|preventDefault={(e) => handleRequestContextMenu(e, request)}
                                                >
                                                    <span class={`mr-2 px-1 text-xs rounded flex-shrink-0 ${getMethodColor(request.method)}`}>
                                                        {request.method}
                                                    </span>
                                                    <div class="flex-1 min-w-0">
                                                        <div class="font-medium truncate text-sm">{request.name || request.path}</div>
                                                        <div class="text-xs text-gray-50 truncate">{request.path}</div>
                                                    </div>
                                                </button>
                                            </div>
                                            
                                            <!-- Examples (when request is expanded) -->
                                            {#if expandedRequests.has(request.id) && request.examples && request.examples.length > 0}
                                                <div class="pl-6 pb-2">
                                                    <div class="text-xs text-gray-50 mb-1">Examples:</div>
                                                    {#each request.examples as example}
                                                        <button 
                                                            class="block w-full text-left px-2 py-1 text-xs rounded-md mb-1 transition-colors {selectedExample && selectedExample.id === example.id ? 'bg-[var(--color-midnight-accent)]/40' : 'bg-[var(--color-midnight-light)] hover:bg-[var(--color-midnight-accent)]/20'}"
                                                            on:click|stopPropagation={() => { selectRequest(request); selectExample(request, example); }}
                                                            on:contextmenu|preventDefault={(e) => handleExampleContextMenu(e, request, example)}
                                                        >
                                                            {example.name}
                                                        </button>
                                                    {/each}
                                                </div>
                                            {/if}
                                        </div>
                                    {/each}
                                {/if}
                            {/each}
                        {:else}
                            <div class="p-2 text-gray-50">No requests found</div>
                        {/if}
                    </div>
                </div>
            {:else}
                <div class="flex flex-col items-center justify-center h-full">
                    <p class="text-center text-gray-50 mb-4">No collections imported</p>
                    <button class="px-4 py-2 bg-[var(--color-midnight-accent)] hover:bg-[var(--color-midnight-accent)]/80 text-[var(--color-button-text)] rounded-md" on:click={importCollection}>
                        Import Collection
                    </button>
                </div>
            {/if}
        </div>
        
        <!-- Request and Response editors -->
        <div class="w-2/3 flex flex-col overflow-hidden">
            {#if selectedRequestId && activeCollection}
                <div class="flex flex-col flex-1 p-4 gap-4 overflow-hidden">
                    <div class="flex items-center justify-between">
                        <div class="flex items-center gap-2">
                            {#if activeRequest}
                                <span class={`px-2 py-1 text-xs rounded ${getMethodColor(activeRequest.method)}`}>
                                    {activeRequest.method}
                                </span>
                                <h2 class="text-xl font-semibold">{activeRequest.name || activeRequest.path}</h2>
                            {/if}
                        </div>
                        <div class="flex items-center gap-2">
                            <button 
                                class="px-3 py-1 text-sm rounded-md hover:bg-[var(--color-midnight-light)]"
                                on:click={() => selectedRequestId && CopyAPIRequestToClipboard(selectedRequestId)}
                            >
                                Copy
                            </button>
                            <button 
                                class="px-3 py-1 text-sm rounded-md hover:bg-[var(--color-midnight-light)] bg-[var(--color-midnight-light)] border border-[var(--color-midnight-darker)]"
                                on:click={addToSelectedFlow}
                            >
                                Add to Current Flow
                            </button>
                        </div>
                    </div>

                    <!-- Split view for request and response -->
                    <div class="flex flex-col flex-1 overflow-hidden gap-4 h-full">
                        <div class="flex-1 min-h-0 h-full">
                            <h3 class="text-lg font-semibold mb-2">Request</h3>
                            <div class="flex-1 h-[calc(100%-25px)] border border-[var(--color-midnight-darker)] rounded-md overflow-hidden bg-[var(--color-midnight)]">
                                {#if requestContent}
                                    {#key monacoRequestKey}
                                        <MonacoEditor
                                            value={requestContent}
                                            language="http"
                                            readOnly={true}
                                            automaticLayout={true}
                                            fontSize={12}
                                            on:change={(e) => requestContent = e.detail}
                                        />
                                    {/key}
                                {:else}
                                    <div class="p-4 text-gray-50 text-center">
                                        Select an example to view request details
                                    </div>
                                {/if}
                            </div>
                        </div>
                        
                        <div class="flex-1 min-h-0 h-full">
                            <h3 class="text-lg font-semibold mb-2">Response</h3>
                            <div class="flex-1 border h-[calc(100%-70px)] border-[var(--color-midnight-darker)] rounded-md overflow-hidden bg-[var(--color-midnight)]">
                                {#if responseContent}
                                    {#key monacoResponseKey}
                                        <MonacoEditor
                                            value={responseContent}
                                            language="http"
                                            readOnly={true}
                                            automaticLayout={true}
                                            fontSize={12}
                                            on:change={(e) => responseContent = e.detail}
                                        />
                                    {/key}
                                {:else}
                                    <div class="p-4 text-gray-50 text-center">
                                        Select an example to view response details
                                    </div>
                                {/if}
                            </div>
                        </div>
                    </div>
                </div>
            {:else}
                <div class="flex items-center justify-center h-full">
                    <p class="text-gray-50">Select a request example to view details</p>
                </div>
            {/if}
        </div>
    </div>
    
    <!-- Context menu -->
    {#if showContextMenu}
        <ContextMenu
            bind:this={contextMenu}
            x={contextMenuX}
            y={contextMenuY}
            onClose={handleContextMenuClose}
            items={getContextMenuItems()}
        />
    {/if}
    
    <!-- Notification -->
    {#if showCopiedNotification}
        <Notification
            bind:this={notification}
            message={notificationMessage}
            type="success"
            duration={3000}
        />
    {/if}
</div>

<style>
    .method-get {
        background-color: rgb(21 128 61);
        color: white;
    }
    
    .method-post {
        background-color: rgb(29 78 216);
        color: white;
    }
    
    .method-put {
        background-color: rgb(202 138 4);
        color: white;
    }
    
    .method-delete {
        background-color: rgb(185 28 28);
        color: white;
    }
    
    .method-patch {
        background-color: rgb(126 34 206);
        color: white;
    }
    
    .method-head {
        background-color: rgb(75 85 99);
        color: white;
    }
    
    .method-options {
        background-color: rgb(107 114 128);
        color: white;
    }
    
    .method-label {
        padding: 0.125rem 0.5rem;
        border-radius: 0.25rem;
        font-size: 0.75rem;
        font-weight: 500;
    }
</style>