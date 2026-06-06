import type { RuntimeClientTarget } from "./writer.js";

export interface ClientState {
  clientId: string;
  selectedSessionId: string;
  connectedAt: number;
}

export interface RequestContext {
  target: RuntimeClientTarget | null;
  clientState?: ClientState;
}

export interface ClientAddOptions {
  selectedSessionId: string;
  requestedClientId?: string;
  now?: number;
}

export class ClientRegistry<T extends RuntimeClientTarget = RuntimeClientTarget> {
  private readonly clients = new Set<T>();
  private readonly states = new Map<T, ClientState>();
  private readonly targetsByClientId = new Map<string, T>();
  private nextClientNumber = 1;

  add(target: T, options: ClientAddOptions | string): ClientState {
    const selectedSessionId = typeof options === "string" ? options : options.selectedSessionId;
    const requestedClientId = typeof options === "string" ? "" : String(options.requestedClientId || "").trim();
    const now = typeof options === "string" ? Date.now() : options.now ?? Date.now();
    const clientId = requestedClientId || target.id || `client_${this.nextClientNumber++}`;
    const previousTarget = this.targetsByClientId.get(clientId);
    if (previousTarget && previousTarget !== target) {
      this.clients.delete(previousTarget);
      this.states.delete(previousTarget);
    }
    const state = { clientId, selectedSessionId, connectedAt: now };
    this.clients.add(target);
    this.states.set(target, state);
    this.targetsByClientId.set(clientId, target);
    return state;
  }

  remove(target: T): ClientState | undefined {
    const state = this.states.get(target);
    this.clients.delete(target);
    this.states.delete(target);
    if (state && this.targetsByClientId.get(state.clientId) === target) {
      this.targetsByClientId.delete(state.clientId);
    }
    return state;
  }

  stateFor(target: T | null | undefined): ClientState | undefined {
    return target ? this.states.get(target) : undefined;
  }

  targetForClientId(clientId: string): T | undefined {
    return this.targetsByClientId.get(clientId);
  }

  reidentify(target: T, requestedClientId: string): ClientState | undefined {
    const clientId = String(requestedClientId || "").trim();
    if (!clientId) return this.stateFor(target);
    const state = this.states.get(target);
    if (!state) return undefined;
    const previousTarget = this.targetsByClientId.get(clientId);
    if (previousTarget && previousTarget !== target) {
      this.clients.delete(previousTarget);
      this.states.delete(previousTarget);
    }
    if (this.targetsByClientId.get(state.clientId) === target) {
      this.targetsByClientId.delete(state.clientId);
    }
    state.clientId = clientId;
    this.targetsByClientId.set(clientId, target);
    return state;
  }

  has(target: T): boolean {
    return this.clients.has(target);
  }

  get size(): number {
    return this.clients.size;
  }

  values(): Iterable<T> {
    return this.clients.values();
  }
}

export function selectedSessionIdForContext(
  context: RequestContext | null | undefined,
  fallbackSessionId: string,
): string {
  return context?.clientState?.selectedSessionId || fallbackSessionId;
}
