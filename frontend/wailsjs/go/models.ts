export namespace main {
	
	export class AppSettings {
	    geminiApiKey: string;
	    ollamaEndpoint: string;
	    openCodeApiKey: string;
	    openRouterApiKey: string;
	    useNativeToolCalls: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.geminiApiKey = source["geminiApiKey"];
	        this.ollamaEndpoint = source["ollamaEndpoint"];
	        this.openCodeApiKey = source["openCodeApiKey"];
	        this.openRouterApiKey = source["openRouterApiKey"];
	        this.useNativeToolCalls = source["useNativeToolCalls"];
	    }
	}
	export class ChatMessage {
	    sender: string;
	    text: string;
	    reasoning: string;
	    imageBase64: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sender = source["sender"];
	        this.text = source["text"];
	        this.reasoning = source["reasoning"];
	        this.imageBase64 = source["imageBase64"];
	    }
	}
	export class FileNode {
	    name: string;
	    path: string;
	    isDir: boolean;
	    children: FileNode[];
	
	    static createFrom(source: any = {}) {
	        return new FileNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.children = this.convertValues(source["children"], FileNode);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModelItem {
	    name: string;
	    isFavorite: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.isFavorite = source["isFavorite"];
	    }
	}
	export class OpenWorkspacesResult {
	    workspaces: string[];
	    activeWorkspace: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenWorkspacesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workspaces = source["workspaces"];
	        this.activeWorkspace = source["activeWorkspace"];
	    }
	}
	export class SavedConversation {
	    sessionId: string;
	    activeModel: string;
	    agentMode: string;
	    yoloMode: boolean;
	    messages: ChatMessage[];
	
	    static createFrom(source: any = {}) {
	        return new SavedConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.activeModel = source["activeModel"];
	        this.agentMode = source["agentMode"];
	        this.yoloMode = source["yoloMode"];
	        this.messages = this.convertValues(source["messages"], ChatMessage);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

