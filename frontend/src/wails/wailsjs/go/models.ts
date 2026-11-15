export namespace config {
	
	export class AppConfig {
	    auto_start: boolean;
	    extra_watch_dirs: string[];
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.auto_start = source["auto_start"];
	        this.extra_watch_dirs = source["extra_watch_dirs"];
	    }
	}

}

export namespace model {
	
	export class CacheSavingsResponse {
	    actual_cost: number;
	    hypothetical_cost: number;
	    saved: number;
	    saved_percent: number;
	    cache_hit_rate: number;
	
	    static createFrom(source: any = {}) {
	        return new CacheSavingsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.actual_cost = source["actual_cost"];
	        this.hypothetical_cost = source["hypothetical_cost"];
	        this.saved = source["saved"];
	        this.saved_percent = source["saved_percent"];
	        this.cache_hit_rate = source["cache_hit_rate"];
	    }
	}
	export class DeviceInfo {
	    id: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.display_name = source["display_name"];
	    }
	}
	export class ModelStats {
	    model: string;
	    events: number;
	    input_tokens: number;
	    cached_tokens: number;
	    cache_creation_tokens: number;
	    output_tokens: number;
	    total_cost: number;
	    cache_hit_rate: number;
	    input_price_per_m: number;
	    cached_price_per_m: number;
	    cache_creation_price_per_m: number;
	    output_price_per_m: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.events = source["events"];
	        this.input_tokens = source["input_tokens"];
	        this.cached_tokens = source["cached_tokens"];
	        this.cache_creation_tokens = source["cache_creation_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.total_cost = source["total_cost"];
	        this.cache_hit_rate = source["cache_hit_rate"];
	        this.input_price_per_m = source["input_price_per_m"];
	        this.cached_price_per_m = source["cached_price_per_m"];
	        this.cache_creation_price_per_m = source["cache_creation_price_per_m"];
	        this.output_price_per_m = source["output_price_per_m"];
	    }
	}
	export class PeriodCost {
	    label: string;
	    cost: number;
	    input_tokens: number;
	    cached_tokens: number;
	    cache_creation_tokens: number;
	    output_tokens: number;
	    cache_hit_rate: number;
	
	    static createFrom(source: any = {}) {
	        return new PeriodCost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.cost = source["cost"];
	        this.input_tokens = source["input_tokens"];
	        this.cached_tokens = source["cached_tokens"];
	        this.cache_creation_tokens = source["cache_creation_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.cache_hit_rate = source["cache_hit_rate"];
	    }
	}
	export class PricingEntry {
	    model: string;
	    input_price_per_m: number;
	    cached_price_per_m: number;
	    cache_creation_price_per_m: number;
	    output_price_per_m: number;

	    static createFrom(source: any = {}) {
	        return new PricingEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model = source["model"];
	        this.input_price_per_m = source["input_price_per_m"];
	        this.cached_price_per_m = source["cached_price_per_m"];
	        this.cache_creation_price_per_m = source["cache_creation_price_per_m"];
	        this.output_price_per_m = source["output_price_per_m"];
	    }
	}
	export class ProjectStat {
	    project: string;
	    events: number;
	    input_tokens: number;
	    cached_tokens: number;
	    cache_creation_tokens: number;
	    output_tokens: number;
	    total_cost: number;
	    cache_hit_rate: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project = source["project"];
	        this.events = source["events"];
	        this.input_tokens = source["input_tokens"];
	        this.cached_tokens = source["cached_tokens"];
	        this.cache_creation_tokens = source["cache_creation_tokens"];
	        this.output_tokens = source["output_tokens"];
	        this.total_cost = source["total_cost"];
	        this.cache_hit_rate = source["cache_hit_rate"];
	    }
	}
	export class SourceStats {
	    name: string;
	    total_input: number;
	    total_cached: number;
	    total_cache_creation: number;
	    total_output: number;
	    total_cost: number;
	    total_events: number;
	    cache_hit_rate: number;
	    models: ModelStats[];
	
	    static createFrom(source: any = {}) {
	        return new SourceStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.total_input = source["total_input"];
	        this.total_cached = source["total_cached"];
	        this.total_cache_creation = source["total_cache_creation"];
	        this.total_output = source["total_output"];
	        this.total_cost = source["total_cost"];
	        this.total_events = source["total_events"];
	        this.cache_hit_rate = source["cache_hit_rate"];
	        this.models = this.convertValues(source["models"], ModelStats);
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
	export class StatsResponse {
	    periods: PeriodCost[];
	    sources: SourceStats[];
	    devices: DeviceInfo[];
	    projects: ProjectStat[];
	    is_paused: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StatsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.periods = this.convertValues(source["periods"], PeriodCost);
	        this.sources = this.convertValues(source["sources"], SourceStats);
	        this.devices = this.convertValues(source["devices"], DeviceInfo);
	        this.projects = this.convertValues(source["projects"], ProjectStat);
	        this.is_paused = source["is_paused"];
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

export namespace updater {
	
	export class Asset {
	    name: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Asset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.url = source["url"];
	    }
	}
	export class Release {
	    tag_name: string;
	    name: string;
	    body: string;
	    assets: Asset[];
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag_name = source["tag_name"];
	        this.name = source["name"];
	        this.body = source["body"];
	        this.assets = this.convertValues(source["assets"], Asset);
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
