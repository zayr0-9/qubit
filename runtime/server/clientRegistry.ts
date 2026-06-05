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

export class ClientRegistry<T extends RuntimeClientTarget = RuntimeClientTarget> {
  private readonly clients = new Set<T>();
  private readonly states = new Map<T, ClientState>();
  private nextClientNumber = 1;

  add(target: T, options: { selectedSessionId: string; now?: number } | string): ClientState {
    const selectedSessionId = typeof options === "string" ? options : options.selectedSessionId;
    const now = typeof options === "string" ? Date.now() : options.now ?? Date.now();
    const clientId = target.id || `client_${this.nextClientNumber++}`;
    const state = { clientId, selectedSessionId, connectedAt: now };
    this.clients.add(target);
    this.states.set(target, state);
    return state;
  }

  remove(target: T): ClientState | undefined {
    const state = this.states.get(target);
    this.clients.delete(target);
    this.states.delete(target);
    return state;
  }

  stateFor(target: T | null | undefined): ClientState | undefined {
    return target ? this.states.get(target) : undefined;
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
