export namespace app {
	
	export class BlameLine {
	    sha: string;
	    author: string;
	    time: number;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new BlameLine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sha = source["sha"];
	        this.author = source["author"];
	        this.time = source["time"];
	        this.summary = source["summary"];
	    }
	}
	export class GitFileStatus {
	    status: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new GitFileStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.path = source["path"];
	    }
	}
	export class GitStatusDTO {
	    branch: string;
	    files: GitFileStatus[];
	
	    static createFrom(source: any = {}) {
	        return new GitStatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.branch = source["branch"];
	        this.files = this.convertValues(source["files"], GitFileStatus);
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
	export class SearchResultDTO {
	    file: string;
	    line: number;
	    col: number;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.line = source["line"];
	        this.col = source["col"];
	        this.text = source["text"];
	    }
	}
	export class Symbol {
	    name: string;
	    kind: string;
	    file: string;
	    importPath: string;
	    pkg: string;
	
	    static createFrom(source: any = {}) {
	        return new Symbol(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.file = source["file"];
	        this.importPath = source["importPath"];
	        this.pkg = source["pkg"];
	    }
	}
	export class ThemeDTO {
	    background: string;
	    foreground: string;
	    border: string;
	    borderFocused: string;
	    accent: string;
	    muted: string;
	    success: string;
	    error: string;
	    warning: string;
	
	    static createFrom(source: any = {}) {
	        return new ThemeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.background = source["background"];
	        this.foreground = source["foreground"];
	        this.border = source["border"];
	        this.borderFocused = source["borderFocused"];
	        this.accent = source["accent"];
	        this.muted = source["muted"];
	        this.success = source["success"];
	        this.error = source["error"];
	        this.warning = source["warning"];
	    }
	}
	export class TreeNodeDTO {
	    name: string;
	    path: string;
	    isDir: boolean;
	    depth: number;
	    expanded: boolean;
	    selected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TreeNodeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	        this.depth = source["depth"];
	        this.expanded = source["expanded"];
	        this.selected = source["selected"];
	    }
	}

}

export namespace commands {
	
	export class SavedCommand {
	    id: string;
	    name: string;
	    cwd: string;
	    command: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedCommand(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.cwd = source["cwd"];
	        this.command = source["command"];
	    }
	}

}

export namespace config {
	
	export class PersistConfig {
	    panel_width: boolean;
	    right_sidebar: boolean;
	    right_panel: boolean;
	    tabs: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PersistConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.panel_width = source["panel_width"];
	        this.right_sidebar = source["right_sidebar"];
	        this.right_panel = source["right_panel"];
	        this.tabs = source["tabs"];
	    }
	}
	export class Config {
	    theme: string;
	    shell: string;
	    format_on_save: boolean;
	    persist?: PersistConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.shell = source["shell"];
	        this.format_on_save = source["format_on_save"];
	        this.persist = this.convertValues(source["persist"], PersistConfig);
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

export namespace process {
	
	export class Process {
	    id: string;
	    pid: number;
	    name: string;
	    cmd: string;
	    cwd: string;
	    // Go type: time
	    start_time: any;
	    ports: number[];
	    cpu_pct: number;
	    mem_mb: number;
	    status: string;
	    exit_code: number;
	
	    static createFrom(source: any = {}) {
	        return new Process(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.pid = source["pid"];
	        this.name = source["name"];
	        this.cmd = source["cmd"];
	        this.cwd = source["cwd"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.ports = source["ports"];
	        this.cpu_pct = source["cpu_pct"];
	        this.mem_mb = source["mem_mb"];
	        this.status = source["status"];
	        this.exit_code = source["exit_code"];
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

export namespace project {
	
	export class Cmd {
	    id: string;
	    name?: string;
	    command: string;
	    directory: string;
	
	    static createFrom(source: any = {}) {
	        return new Cmd(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.directory = source["directory"];
	    }
	}
	export class RecentEntry {
	    path: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new RecentEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	    }
	}
	export class SavedTab {
	    type: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SavedTab(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.path = source["path"];
	    }
	}
	export class UIState {
	    right_width?: number;
	    show_right?: boolean;
	    tabs?: SavedTab[];
	    active_tab?: string;
	    right_panel?: string;
	
	    static createFrom(source: any = {}) {
	        return new UIState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.right_width = source["right_width"];
	        this.show_right = source["show_right"];
	        this.tabs = this.convertValues(source["tabs"], SavedTab);
	        this.active_tab = source["active_tab"];
	        this.right_panel = source["right_panel"];
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

