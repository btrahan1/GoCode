import React, { useState, useEffect, useRef } from 'react';
import './App.css';
import { 
  SelectWorkspace, 
  LoadSettings, 
  SaveSettings, 
  LoadConversation, 
  SendUserMessage, 
  CancelAgent,
  GetDirectoryTree,
  GetFileContent,
  SaveFileContent,
  ListConversations,
  LoadSpecificConversation,
  CreateNewConversation,
  GetOpenWorkspaces,
  SaveOpenWorkspaces,
  GetModelList,
  ToggleModelFavorite,
  OpenPathInExplorer
} from '../wailsjs/go/main/App';
import * as runtime from '../wailsjs/runtime/runtime';

interface ChatMessage {
  sender: string;
  text: string;
  reasoning?: string;
  imageBase64?: string;
}

interface AppSettings {
  geminiApiKey: string;
  ollamaEndpoint: string;
  openCodeApiKey: string;
  openRouterApiKey: string;
  useNativeToolCalls: boolean;
  sidebarWidth: number;
  logPanelWidth: number;
}

interface FileNode {
  name: string;
  path: string;
  isDir: boolean;
  children?: FileNode[];
}

interface ConversationHeader {
  sessionId: string;
  activeModel: string;
  agentMode: string;
  yoloMode: boolean;
}

interface ModelItem {
  name: string;
  isFavorite: boolean;
}

// Recursive Tree Node Component
interface TreeNodeProps {
  node: FileNode;
  selectedPath: string;
  checkedPaths: { [key: string]: boolean };
  onSelectFile: (path: string) => void;
  onToggleCheck: (node: FileNode, checked: boolean) => void;
  onRightClick: (e: React.MouseEvent, node: FileNode) => void;
}

function TreeNode({ node, selectedPath, checkedPaths, onSelectFile, onToggleCheck, onRightClick }: TreeNodeProps) {
  const [isExpanded, setIsExpanded] = useState<boolean>(false);

  const handleToggle = (e: React.MouseEvent) => {
    // Ignore toggles clicking checkboxes directly
    if ((e.target as HTMLElement).tagName === 'INPUT') return;
    if (node.isDir) {
      setIsExpanded(!isExpanded);
    } else {
      onSelectFile(node.path);
    }
  };

  const isSelected = selectedPath === node.path;
  const isChecked = !!checkedPaths[node.path];

  const handleCheckboxChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onToggleCheck(node, e.target.checked);
  };

  const handleContextMenu = (e: React.MouseEvent) => {
    e.preventDefault();
    onRightClick(e, node);
  };

  return (
    <div className="tree-node">
      <div 
        className={`tree-node-label ${isSelected ? 'selected' : ''}`}
        onClick={handleToggle}
        onContextMenu={handleContextMenu}
      >
        <input 
          type="checkbox" 
          className="tree-node-checkbox" 
          checked={isChecked}
          onChange={handleCheckboxChange}
        />
        <span className="tree-node-icon">
          {node.isDir ? (isExpanded ? '📂' : '📁') : '📄'}
        </span>
        <span>{node.name}</span>
      </div>

      {node.isDir && isExpanded && node.children && (
        <div className="tree-node-children">
          {node.children.map((child, idx) => (
            <TreeNode 
              key={idx} 
              node={child} 
              selectedPath={selectedPath} 
              checkedPaths={checkedPaths}
              onSelectFile={onSelectFile}
              onToggleCheck={onToggleCheck}
              onRightClick={onRightClick}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function App() {
  const [activeModel, setActiveModel] = useState<string>('gemini-2.5-flash');
  const [agentMode, setAgentMode] = useState<string>('coder');
  const [yoloMode, setYoloMode] = useState<boolean>(false);
  
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [inputValue, setInputValue] = useState<string>('');
  const [logs, setLogs] = useState<string>('System initialized.');
  const [agentStatus, setAgentStatus] = useState<{ status: string; color: string }>({ status: 'Ready', color: 'green' });
  const [isGenerating, setIsGenerating] = useState<boolean>(false);

  // Multiple project workspaces
  const [openWorkspaces, setOpenWorkspaces] = useState<string[]>([]);
  const [activeWorkspace, setActiveWorkspace] = useState<string>('');

  // File explorer & editor states
  const [treeData, setTreeData] = useState<FileNode | null>(null);
  const [selectedFilePath, setSelectedFilePath] = useState<string>('');
  const [editorContent, setEditorContent] = useState<string>('');
  const [activeTab, setActiveTab] = useState<'chat' | 'editor'>('chat');
  const [isSaving, setIsSaving] = useState<boolean>(false);

  // Multiple Chat states
  const [chatList, setChatList] = useState<ConversationHeader[]>([]);
  const [activeSessionId, setActiveSessionId] = useState<string>('');

  // Model list and favorites
  const [modelsList, setModelsList] = useState<ModelItem[]>([]);

  // Sizable Panel Widths
  const [sidebarWidth, setSidebarWidth] = useState<number>(280);
  const [logPanelWidth, setLogPanelWidth] = useState<number>(320);

  const sidebarResizingRef = useRef<boolean>(false);
  const logPanelResizingRef = useRef<boolean>(false);
  const sidebarWidthRef = useRef<number>(280);
  const logPanelWidthRef = useRef<number>(320);

  // Checkboxes in FileTree
  const [checkedPaths, setCheckedPaths] = useState<{ [path: string]: boolean }>({});
  const [isCopying, setIsCopying] = useState<boolean>(false);

  // Context Menu State
  const [contextMenu, setContextMenu] = useState<{
    visible: boolean;
    x: number;
    y: number;
    node: FileNode | null;
  }>({ visible: false, x: 0, y: 0, node: null });

  // Settings
  const [showSettings, setShowSettings] = useState<boolean>(false);
  const [settings, setSettings] = useState<AppSettings>({
    geminiApiKey: '',
    ollamaEndpoint: 'http://localhost:11434',
    openCodeApiKey: '',
    openRouterApiKey: '',
    useNativeToolCalls: false,
    sidebarWidth: 280,
    logPanelWidth: 320
  });

  const chatEndRef = useRef<HTMLDivElement>(null);
  const logsEndRef = useRef<HTMLDivElement>(null);

  // Auto-scroll chat
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Auto-scroll logs
  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs]);

  // Dismiss context menu on left click anywhere
  useEffect(() => {
    const handleGlobalClick = () => {
      if (contextMenu.visible) {
        setContextMenu({ visible: false, x: 0, y: 0, node: null });
      }
    };
    document.addEventListener('click', handleGlobalClick);
    return () => document.removeEventListener('click', handleGlobalClick);
  }, [contextMenu]);

  // Load settings on startup
  useEffect(() => {
    LoadSettings()
      .then((loadedSettings) => {
        setSettings(loadedSettings);
        if (loadedSettings.sidebarWidth) {
          setSidebarWidth(loadedSettings.sidebarWidth);
          sidebarWidthRef.current = loadedSettings.sidebarWidth;
        }
        if (loadedSettings.logPanelWidth) {
          setLogPanelWidth(loadedSettings.logPanelWidth);
          logPanelWidthRef.current = loadedSettings.logPanelWidth;
        }
        // Refresh models list after settings load (Ollama endpoint might change)
        refreshModelsList();
      })
      .catch((err) => {
        console.error("Failed to load settings:", err);
      });
  }, []);

  // Restore open workspaces on mount
  useEffect(() => {
    GetOpenWorkspaces()
      .then((res) => {
        if (res && res.workspaces && res.workspaces.length > 0) {
          setOpenWorkspaces(res.workspaces);
          if (res.activeWorkspace && res.workspaces.includes(res.activeWorkspace)) {
            setActiveWorkspace(res.activeWorkspace);
          } else {
            setActiveWorkspace(res.workspaces[0]);
          }
        }
      })
      .catch((err) => {
        console.error("Failed to restore open workspaces:", err);
      });
  }, []);

  // Set up event listeners for agent loop updates
  useEffect(() => {
    const handleStatus = (data: { status: string; color: string }) => {
      setAgentStatus(data);
      if (data.status === 'Ready') {
        setIsGenerating(false);
      } else {
        setIsGenerating(true);
      }
    };

    const handleLog = (text: string) => {
      setLogs((prev) => prev + '\n' + text);
    };

    const handleLogStream = (text: string) => {
      setLogs((prev) => prev + text);
    };

    const handleMessage = (msg: ChatMessage) => {
      setMessages((prev) => [...prev, msg]);
    };

    const handleComplete = () => {
      setIsGenerating(false);
      setAgentStatus({ status: 'Ready', color: 'green' });
      
      // Reload active conversation and refresh tree
      if (activeSessionId) {
        LoadSpecificConversation(activeSessionId).then((conv) => {
          if (conv) {
            setMessages(conv.messages);
          }
        });
      }
      refreshTree();
    };

    const handleError = (errText: string) => {
      setLogs((prev) => prev + '\n[ERROR] ' + errText);
      setIsGenerating(false);
      setAgentStatus({ status: 'Error', color: 'red' });
    };

    runtime.EventsOn('agent:status', handleStatus);
    runtime.EventsOn('agent:log', handleLog);
    runtime.EventsOn('agent:log_stream', handleLogStream);
    runtime.EventsOn('agent:message', handleMessage);
    runtime.EventsOn('agent:complete', handleComplete);
    runtime.EventsOn('agent:error', handleError);

    return () => {
      runtime.EventsOff('agent:status');
      runtime.EventsOff('agent:log');
      runtime.EventsOff('agent:log_stream');
      runtime.EventsOff('agent:message');
      runtime.EventsOff('agent:complete');
      runtime.EventsOff('agent:error');
    };
  }, [activeSessionId, activeWorkspace]);

  // Load conversation history headers and last conversation when active workspace changes
  useEffect(() => {
    if (activeWorkspace) {
      setLogs(`Switched project workspace: ${activeWorkspace}`);
      setSelectedFilePath('');
      setEditorContent('');
      setActiveTab('chat');
      setCheckedPaths({});
      
      // Refresh chat list & models list
      refreshChatList();
      refreshModelsList();

      // Load last conversation to display first
      LoadConversation(activeWorkspace)
        .then((conv) => {
          if (conv) {
            setActiveSessionId(conv.sessionId);
            setActiveModel(conv.activeModel);
            setAgentMode(conv.agentMode);
            setYoloMode(conv.yoloMode);
            setMessages(conv.messages);
          } else {
            setMessages([]);
            setActiveSessionId('');
          }
        })
        .catch((err) => {
          setLogs((prev) => prev + `\nFailed to load conversation: ${err}`);
        });

      // Load file tree
      refreshTree();
    } else {
      setTreeData(null);
      setMessages([]);
      setChatList([]);
      setActiveSessionId('');
    }
  }, [activeWorkspace]);

  // Sidebar drag resizer logic
  const handleSidebarMouseMove = (e: MouseEvent) => {
    if (!sidebarResizingRef.current) return;
    const newWidth = Math.max(200, Math.min(550, e.clientX));
    sidebarWidthRef.current = newWidth;
    setSidebarWidth(newWidth);
  };

  const stopSidebarResize = () => {
    sidebarResizingRef.current = false;
    document.removeEventListener('mousemove', handleSidebarMouseMove);
    document.removeEventListener('mouseup', stopSidebarResize);
    
    // Save to settings
    setSettings((prev) => {
      const next = { ...prev, sidebarWidth: sidebarWidthRef.current };
      SaveSettings(next);
      return next;
    });
  };

  const startSidebarResize = (e: React.MouseEvent) => {
    e.preventDefault();
    sidebarResizingRef.current = true;
    document.addEventListener('mousemove', handleSidebarMouseMove);
    document.addEventListener('mouseup', stopSidebarResize);
  };

  // Log panel drag resizer logic
  const handleLogPanelMouseMove = (e: MouseEvent) => {
    if (!logPanelResizingRef.current) return;
    const newWidth = Math.max(200, Math.min(600, window.innerWidth - e.clientX));
    logPanelWidthRef.current = newWidth;
    setLogPanelWidth(newWidth);
  };

  const stopLogPanelResize = () => {
    logPanelResizingRef.current = false;
    document.removeEventListener('mousemove', handleLogPanelMouseMove);
    document.removeEventListener('mouseup', stopLogPanelResize);
    
    // Save to settings
    setSettings((prev) => {
      const next = { ...prev, logPanelWidth: logPanelWidthRef.current };
      SaveSettings(next);
      return next;
    });
  };

  const startLogPanelResize = (e: React.MouseEvent) => {
    e.preventDefault();
    logPanelResizingRef.current = true;
    document.addEventListener('mousemove', handleLogPanelMouseMove);
    document.addEventListener('mouseup', stopLogPanelResize);
  };

  // Tree nodes checklist helpers
  const toggleNodeAndChildren = (node: FileNode, checkState: boolean, currentMap: { [p: string]: boolean }) => {
    currentMap[node.path] = checkState;
    if (node.isDir && node.children) {
      node.children.forEach((child) => {
        toggleNodeAndChildren(child, checkState, currentMap);
      });
    }
  };

  const handleToggleCheck = (node: FileNode, checked: boolean) => {
    setCheckedPaths((prev) => {
      const nextMap = { ...prev };
      toggleNodeAndChildren(node, checked, nextMap);
      return nextMap;
    });
  };

  const handleRightClickNode = (e: React.MouseEvent, node: FileNode) => {
    setContextMenu({
      visible: true,
      x: e.clientX,
      y: e.clientY,
      node
    });
  };

  const handleOpenInExplorer = () => {
    if (!contextMenu.node || !activeWorkspace) return;
    OpenPathInExplorer(activeWorkspace, contextMenu.node.path)
      .then(() => {
        setLogs((prev) => prev + `\nOpened in Explorer: ${contextMenu.node?.path}`);
      })
      .catch((err) => {
        alert(`Failed to open in Explorer: ${err}`);
      });
  };

  // Recurse to find all checked files (leaves)
  const getCheckedFilesList = (node: FileNode, map: { [p: string]: boolean }, list: string[]) => {
    if (!node.isDir) {
      if (map[node.path]) {
        list.push(node.path);
      }
    } else if (node.children) {
      node.children.forEach((child) => getCheckedFilesList(child, map, list));
    }
  };

  const handleCopySelected = async () => {
    if (!treeData || !activeWorkspace) return;
    const filePaths: string[] = [];
    getCheckedFilesList(treeData, checkedPaths, filePaths);

    if (filePaths.length === 0) {
      alert("No files checked! Check boxes next to files in the tree view to select them first.");
      return;
    }

    setIsCopying(true);
    let clipboardBuffer = "";

    try {
      for (const path of filePaths) {
        const content = await GetFileContent(activeWorkspace, path);
        clipboardBuffer += `File: ${path}\n\`\`\`\n${content}\n\`\`\`\n\n`;
      }
      await navigator.clipboard.writeText(clipboardBuffer);
      setLogs((prev) => prev + `\n[CLIPBOARD] Copied content of ${filePaths.length} selected files to clipboard.`);
    } catch (err) {
      alert(`Failed to copy selected files: ${err}`);
    } finally {
      setIsCopying(false);
    }
  };

  const refreshModelsList = () => {
    GetModelList()
      .then((list) => {
        setModelsList(list || []);
      })
      .catch((err) => {
        console.error("Failed to load models list:", err);
      });
  };

  const refreshChatList = () => {
    if (!activeWorkspace) return;
    ListConversations(activeWorkspace)
      .then((list) => {
        setChatList(list || []);
      })
      .catch((err) => {
        console.error("Failed to load chat history:", err);
      });
  };

  const refreshTree = () => {
    if (!activeWorkspace) return;
    GetDirectoryTree(activeWorkspace)
      .then((tree) => {
        setTreeData(tree);
      })
      .catch((err) => {
        setLogs((prev) => prev + `\nFailed to load directory tree: ${err}`);
      });
  };

  const handleOpenWorkspace = async () => {
    try {
      const dir = await SelectWorkspace();
      if (dir) {
        if (!openWorkspaces.includes(dir)) {
          const newList = [...openWorkspaces, dir];
          setOpenWorkspaces(newList);
          setActiveWorkspace(dir);
          SaveOpenWorkspaces(newList, dir);
        } else {
          setActiveWorkspace(dir);
          SaveOpenWorkspaces(openWorkspaces, dir);
        }
      }
    } catch (err) {
      console.error(err);
    }
  };

  const handleCloseWorkspace = (path: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const newList = openWorkspaces.filter((w) => w !== path);
    setOpenWorkspaces(newList);
    
    let nextActive = activeWorkspace;
    if (activeWorkspace === path) {
      nextActive = newList.length > 0 ? newList[0] : '';
      setActiveWorkspace(nextActive);
    }
    SaveOpenWorkspaces(newList, nextActive);
  };

  const handleSelectWorkspaceTab = (path: string) => {
    setActiveWorkspace(path);
    SaveOpenWorkspaces(openWorkspaces, path);
  };

  const handleNewChat = () => {
    if (!activeWorkspace) return;
    CreateNewConversation(activeWorkspace, activeModel, agentMode, yoloMode)
      .then((newConv) => {
        setActiveSessionId(newConv.sessionId);
        setMessages([]);
        refreshChatList();
        setLogs((prev) => prev + `\nCreated new chat session: ${newConv.sessionId}`);
      })
      .catch((err) => {
        alert(`Failed to create new chat: ${err}`);
      });
  };

  const handleSelectChat = (sessionId: string) => {
    LoadSpecificConversation(sessionId)
      .then((conv) => {
        if (conv) {
          setActiveSessionId(conv.sessionId);
          setActiveModel(conv.activeModel);
          setAgentMode(conv.agentMode);
          setYoloMode(conv.yoloMode);
          setMessages(conv.messages);
          setLogs((prev) => prev + `\nSwitched to chat session: ${sessionId}`);
        }
      })
      .catch((err) => {
        alert(`Failed to load chat: ${err}`);
      });
  };

  const handleSendMessage = async () => {
    if (!inputValue.trim() || !activeWorkspace) return;

    const userText = inputValue;
    setInputValue('');

    // Pre-insert user bubble to give immediate feedback
    setMessages((prev) => [...prev, { sender: 'user', text: userText }]);
    setIsGenerating(true);

    try {
      await SendUserMessage(activeWorkspace, activeSessionId, userText, activeModel, agentMode, yoloMode);
      setTimeout(refreshChatList, 1000);
    } catch (err) {
      setLogs((prev) => prev + `\n[ERROR] Failed to send message: ${err}`);
      setIsGenerating(false);
    }
  };

  const handleCancelAgent = () => {
    CancelAgent();
    setIsGenerating(false);
  };

  const handleToggleFavorite = () => {
    if (!activeModel) return;
    ToggleModelFavorite(activeModel)
      .then(() => {
        refreshModelsList();
        setLogs((prev) => prev + `\nToggled favorite status for: ${activeModel}`);
      })
      .catch((err) => {
        alert(`Failed to toggle favorite: ${err}`);
      });
  };

  const handleSaveSettings = async () => {
    try {
      await SaveSettings(settings);
      setShowSettings(false);
      setLogs((prev) => prev + '\nSettings saved successfully.');
      // Refresh models list in case Ollama endpoint changed
      refreshModelsList();
    } catch (err) {
      alert(`Failed to save settings: ${err}`);
    }
  };

  // Open file in Editor tab
  const handleSelectFile = (path: string) => {
    setSelectedFilePath(path);
    GetFileContent(activeWorkspace, path)
      .then((content) => {
        setEditorContent(content);
        setActiveTab('editor');
      })
      .catch((err) => {
        alert(`Failed to load file: ${err}`);
      });
  };

  // Save modified file content
  const handleSaveFileContent = () => {
    if (!selectedFilePath || !activeWorkspace) return;
    setIsSaving(true);
    SaveFileContent(activeWorkspace, selectedFilePath, editorContent)
      .then(() => {
        setLogs((prev) => prev + `\nSaved file: ${selectedFilePath}`);
        setIsSaving(false);
      })
      .catch((err) => {
        alert(`Failed to save file: ${err}`);
        setIsSaving(false);
      });
  };

  // Format short display names for session IDs
  const getSessionDisplayName = (header: ConversationHeader) => {
    const parts = header.sessionId.split('-');
    if (parts.length > 1) {
      const ts = parseInt(parts[1], 10);
      if (!isNaN(ts)) {
        const date = new Date(ts / 1000000);
        return `Chat - ${date.toLocaleDateString()} ${date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
      }
    }
    return `Chat (${header.activeModel})`;
  };

  // Determine if active model is a favorite
  const isCurrentModelFavorite = modelsList.find((m) => m.name === activeModel)?.isFavorite || false;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '100vw' }}>
      {/* Top Workspace Tab Bar */}
      <div className="workspace-tabs">
        {openWorkspaces.map((path) => {
          const isActive = activeWorkspace === path;
          const displayFolderName = path.split('\\').pop() || path.split('/').pop() || path;
          return (
            <div 
              key={path}
              className={`workspace-tab ${isActive ? 'active' : ''}`}
              onClick={() => handleSelectWorkspaceTab(path)}
              title={path}
            >
              <span>📁 {displayFolderName}</span>
              <button 
                className="workspace-tab-close"
                onClick={(e) => handleCloseWorkspace(path, e)}
              >
                ×
              </button>
            </div>
          );
        })}
        <button className="workspace-tab-add" onClick={handleOpenWorkspace}>
          + Open Workspace
        </button>
      </div>

      {/* Horizontal Configuration Toolbar */}
      {activeWorkspace && (
        <div className="controls-toolbar">
          <div className="toolbar-group">
            <span className="toolbar-label">Model:</span>
            <select 
              className="select-control-sm" 
              value={activeModel}
              onChange={(e) => setActiveModel(e.target.value)}
            >
              {modelsList.map((item, idx) => (
                <option key={idx} value={item.name}>
                  {item.isFavorite ? `★ ${item.name}` : item.name}
                </option>
              ))}
            </select>
            <button 
              className="btn-star-favorite-sm" 
              onClick={handleToggleFavorite}
              title="Toggle Favorite Model"
            >
              {isCurrentModelFavorite ? '★' : '☆'}
            </button>
          </div>

          <div className="toolbar-group">
            <span className="toolbar-label">Agent Mode:</span>
            <select 
              className="select-control-sm"
              value={agentMode}
              onChange={(e) => setAgentMode(e.target.value)}
            >
              <option value="coder">Coder Loop (Auto)</option>
              <option value="chat">Chat Mode (No Tools)</option>
            </select>
          </div>

          <div className="toolbar-group">
            <div className="toolbar-toggle-container">
              <span className="toolbar-label">YOLO Mode:</span>
              <label className="toggle-switch">
                <input 
                  type="checkbox" 
                  checked={yoloMode}
                  onChange={(e) => setYoloMode(e.target.checked)}
                />
                <span className="slider"></span>
              </label>
            </div>
          </div>

          <div className="toolbar-group" style={{ marginLeft: 'auto' }}>
            <button 
              className="btn-gear-settings" 
              onClick={() => setShowSettings(true)}
              title="Configure API Keys & Endpoints"
            >
              ⚙️
            </button>
          </div>
        </div>
      )}

      {/* Main Workspace Workspace Layout */}
      <div className="app-container" style={{ flex: 1, height: activeWorkspace ? 'calc(100vh - 83px)' : 'calc(100vh - 39px)' }}>
        {/* Sidebar Panel */}
        <div 
          className="sidebar" 
          style={{ 
            width: `${sidebarWidth}px`, 
            minWidth: `${sidebarWidth}px`, 
            maxWidth: `${sidebarWidth}px` 
          }}
        >
          {activeWorkspace ? (
            <>
              <div className="workspace-section" style={{ maxHeight: '180px', display: 'flex', flexDirection: 'column' }}>
                <span className="section-label">Chat History</span>
                <button className="btn-new-chat" onClick={handleNewChat}>
                  + New Chat Thread
                </button>
                <div className="chat-list-container">
                  {chatList.map((header) => {
                    const isActive = activeSessionId === header.sessionId;
                    return (
                      <div 
                        key={header.sessionId}
                        className={`chat-list-item ${isActive ? 'active' : ''}`}
                        onClick={() => handleSelectChat(header.sessionId)}
                      >
                        <div>
                          <div>{getSessionDisplayName(header)}</div>
                          <div className="chat-item-meta">{header.activeModel}</div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>

              <div className="workspace-section" style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: '120px' }}>
                <div className="explorer-header">
                  <span className="section-label" style={{ marginBottom: 0 }}>File Explorer</span>
                  <button 
                    className="btn-copy-selected"
                    onClick={handleCopySelected}
                    disabled={isCopying}
                    title="Copy selected files code content into clipboard"
                  >
                    📋 {isCopying ? 'Copying...' : 'Copy Selected'}
                  </button>
                </div>
                {treeData ? (
                  <div className="file-tree-container">
                    <TreeNode 
                      node={treeData} 
                      selectedPath={selectedFilePath} 
                      checkedPaths={checkedPaths}
                      onSelectFile={handleSelectFile}
                      onToggleCheck={handleToggleCheck}
                      onRightClick={handleRightClickNode}
                    />
                  </div>
                ) : (
                  <div style={{ color: 'var(--text-muted)', fontSize: '12px', padding: '10px', textAlign: 'center' }}>
                    Loading file structure...
                  </div>
                )}
              </div>
            </>
          ) : (
            <div style={{ color: 'var(--text-muted)', fontSize: '13px', textAlign: 'center', marginTop: '20px' }}>
              No project workspaces open. Click "+ Open Workspace" above to load one!
            </div>
          )}
        </div>

        {/* Sidebar Drag Resizer */}
        <div className="resizer-bar" onMouseDown={startSidebarResize} />

        {/* Main Panel with Tabs */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column' }}>
          {activeWorkspace ? (
            <>
              <div className="tab-bar">
                <button 
                  className={`tab-btn ${activeTab === 'chat' ? 'active' : ''}`}
                  onClick={() => setActiveTab('chat')}
                >
                  💬 Agent Chat
                </button>
                <button 
                  className={`tab-btn ${activeTab === 'editor' ? 'active' : ''}`}
                  disabled={!selectedFilePath}
                  onClick={() => setActiveTab('editor')}
                >
                  📝 Code Editor {selectedFilePath && `(${selectedFilePath.split('/').pop()})`}
                </button>
              </div>

              {activeTab === 'chat' ? (
                <div className="chat-panel" style={{ height: 'calc(100vh - 126px)' }}>
                  <div className="chat-header">
                    <div className="status-indicator">
                      <div className="status-dot" style={{ color: agentStatus.color, backgroundColor: agentStatus.color }}></div>
                      <span>Status: {agentStatus.status}</span>
                    </div>

                    {isGenerating && (
                      <button className="btn-cancel-agent" onClick={handleCancelAgent}>
                        Cancel Agent
                      </button>
                    )}
                  </div>

                  <div className="chat-messages">
                    {messages.length === 0 ? (
                      <div style={{ color: 'var(--text-muted)', textAlign: 'center', marginTop: '40px' }}>
                        No messages yet. Send a prompt to begin!
                      </div>
                    ) : (
                      messages.map((msg, i) => {
                        const isToolOutput = msg.text.startsWith('### TOOL OUTPUT:');
                        return (
                          <div key={i} className={`message-bubble ${msg.sender}`}>
                            <span className="bubble-sender">{msg.sender}</span>
                            {isToolOutput ? (
                              <div className="tool-execution-card">
                                {msg.text}
                              </div>
                            ) : (
                              <div className="bubble-content">
                                {msg.text}
                              </div>
                            )}
                          </div>
                        );
                      })
                    )}
                    <div ref={chatEndRef} />
                  </div>

                  <div className="input-container">
                    <textarea
                      className="chat-input"
                      placeholder="Type a prompt to run the agent loop..."
                      disabled={isGenerating}
                      value={inputValue}
                      onChange={(e) => setInputValue(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && !e.shiftKey) {
                          e.preventDefault();
                          handleSendMessage();
                        }
                      }}
                    />
                    <button 
                      className="btn-send" 
                      disabled={isGenerating || !inputValue.trim()}
                      onClick={handleSendMessage}
                    >
                      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                        <line x1="22" y1="2" x2="11" y2="13"></line>
                        <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
                      </svg>
                    </button>
                  </div>
                </div>
              ) : (
                <div className="editor-panel" style={{ height: 'calc(100vh - 126px)' }}>
                  <div className="editor-header">
                    <span className="editor-filename">Selected File: {selectedFilePath}</span>
                  </div>
                  <textarea
                    className="editor-textarea"
                    value={editorContent}
                    onChange={(e) => setEditorContent(e.target.value)}
                  />
                  <div className="editor-actions">
                    <button 
                      className="btn-primary" 
                      disabled={isSaving}
                      onClick={handleSaveFileContent}
                    >
                      {isSaving ? 'Saving Changes...' : '💾 Save Changes'}
                    </button>
                  </div>
                </div>
              )}
            </>
          ) : (
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%', color: 'var(--text-muted)' }}>
              No project workspaces open. Click "+ Open Workspace" above to select a folder!
            </div>
          )}
        </div>

        {/* Log Panel Drag Resizer */}
        <div className="resizer-bar" onMouseDown={startLogPanelResize} />

        {/* Telemetry & Shell Logging Panel */}
        <div 
          className="log-panel" 
          style={{ 
            height: '100%',
            width: `${logPanelWidth}px`,
            minWidth: `${logPanelWidth}px`,
            maxWidth: `${logPanelWidth}px`
          }}
        >
          <div className="log-header">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <polyline points="4 17 10 11 4 5"></polyline>
              <line x1="12" y1="19" x2="20" y2="19"></line>
            </svg>
            <span>Terminal & Telemetry</span>
          </div>
          <div className="log-content">
            {logs}
            <div ref={logsEndRef} />
          </div>
        </div>
      </div>

      {/* Floating Context Menu */}
      {contextMenu.visible && (
        <div 
          className="context-menu" 
          style={{ top: `${contextMenu.y}px`, left: `${contextMenu.x}px` }}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="context-menu-item" onClick={handleOpenInExplorer}>
            📂 Open in Explorer
          </div>
        </div>
      )}

      {/* Settings Modal */}
      {showSettings && (
        <div className="modal-overlay">
          <div className="modal-content">
            <span className="modal-title">API & Endpoint Configuration</span>
            
            <div className="form-group">
              <label className="section-label">Gemini API Key</label>
              <input
                type="password"
                className="form-input"
                placeholder="AIzaSy..."
                value={settings.geminiApiKey}
                onChange={(e) => setSettings({ ...settings, geminiApiKey: e.target.value })}
              />
            </div>

            <div className="form-group">
              <label className="section-label">Ollama Endpoint</label>
              <input
                type="text"
                className="form-input"
                value={settings.ollamaEndpoint}
                onChange={(e) => setSettings({ ...settings, ollamaEndpoint: e.target.value })}
              />
            </div>

            <div className="form-group">
              <label className="section-label">OpenCode API Key</label>
              <input
                type="password"
                className="form-input"
                placeholder="sk-..."
                value={settings.openCodeApiKey}
                onChange={(e) => setSettings({ ...settings, openCodeApiKey: e.target.value })}
              />
            </div>

            <div className="form-group">
              <label className="section-label">OpenRouter API Key</label>
              <input
                type="password"
                className="form-input"
                placeholder="sk-or-..."
                value={settings.openRouterApiKey}
                onChange={(e) => setSettings({ ...settings, openRouterApiKey: e.target.value })}
              />
            </div>

            <div className="form-group">
              <label className="toggle-container">
                <span className="section-label" style={{ marginBottom: 0 }}>Use Native Tool Calls</span>
                <label className="toggle-switch">
                  <input
                    type="checkbox"
                    checked={settings.useNativeToolCalls}
                    onChange={(e) => setSettings({ ...settings, useNativeToolCalls: e.target.checked })}
                  />
                  <span className="slider"></span>
                </label>
              </label>
            </div>

            <div className="modal-actions">
              <button className="btn-secondary" onClick={() => setShowSettings(false)}>Cancel</button>
              <button className="btn-primary" onClick={handleSaveSettings}>Save Configuration</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;
