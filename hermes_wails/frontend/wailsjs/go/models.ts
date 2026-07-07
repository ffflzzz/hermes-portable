export namespace main {
	
	export class ChatMsg {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMsg(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}

}

