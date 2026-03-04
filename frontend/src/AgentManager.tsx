import { useState, useEffect, useCallback } from 'react';
import ReactFlow, { 
  MiniMap, 
  Controls, 
  Background, 
  useNodesState, 
  useEdgesState, 
  addEdge, 
  type Node, 
  type Edge,
  type Connection
} from 'reactflow';
import 'reactflow/dist/style.css';
import { Plus, Trash2, Edit2, ArrowLeft, Save } from 'lucide-react';

const initialNodes: Node[] = [
  { id: '1', position: { x: 100, y: 100 }, data: { label: 'Input Node' }, type: 'input' },
  { id: '2', position: { x: 300, y: 100 }, data: { label: 'Chat Model' } },
  { id: '3', position: { x: 500, y: 100 }, data: { label: 'Output Node' }, type: 'output' },
];

const initialEdges: Edge[] = [
  { id: 'e1-2', source: '1', target: '2' },
  { id: 'e2-3', source: '2', target: '3' },
];

export const AgentManager = () => {
  const [agents, setAgents] = useState<any[]>([]);
  const [view, setView] = useState<'list' | 'edit'>('list');
  const [currentAgent, setCurrentAgent] = useState<any>(null);

  const [nodes, setNodes, onNodesChange] = useNodesState([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);

  const fetchAgents = async () => {
    try {
      const res = await fetch('/api/agents');
      if (res.ok) {
        const data = await res.json();
        setAgents(data || []);
      }
    } catch (e) {
      console.error('Failed to fetch agents', e);
    }
  };

  useEffect(() => {
    if (view === 'list') {
      fetchAgents();
    }
  }, [view]);

  const handleCreateNew = () => {
    setCurrentAgent({ name: 'New Agent', description: 'A new AI agent' });
    setNodes(initialNodes);
    setEdges(initialEdges);
    setView('edit');
  };

  const handleEdit = (agent: any) => {
    setCurrentAgent(agent);
    try {
      const schema = JSON.parse(agent.schemaJson);
      setNodes(schema.nodes || []);
      setEdges(schema.edges || []);
    } catch (e) {
      setNodes(initialNodes);
      setEdges(initialEdges);
    }
    setView('edit');
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Are you sure you want to delete this agent?')) return;
    try {
      await fetch(`/api/agents?id=${id}`, { method: 'DELETE' });
      fetchAgents();
    } catch (e) {
      console.error('Delete failed', e);
    }
  };

  const handleSave = async () => {
    const schemaJson = JSON.stringify({ nodes, edges });
    const payload = { ...currentAgent, schemaJson };
    try {
      const res = await fetch('/api/agents', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        const saved = await res.json();
        setCurrentAgent(saved);
        alert('Agent saved successfully!');
      }
    } catch (e) {
      console.error('Save failed', e);
    }
  };

  const onConnect = useCallback((params: Edge | Connection) => setEdges((eds) => addEdge(params, eds)), [setEdges]);

  if (view === 'edit') {
    return (
      <div className="flex flex-col h-full bg-background text-foreground animate-in fade-in duration-200">
        <div className="h-14 border-b border-border flex items-center px-4 justify-between bg-card/50">
          <div className="flex items-center gap-4">
            <button onClick={() => setView('list')} className="p-2 hover:bg-accent rounded-md transition-colors">
              <ArrowLeft className="w-5 h-5 text-muted-foreground" />
            </button>
            <div className="flex flex-col">
              <input 
                type="text" 
                value={currentAgent?.name || ''} 
                onChange={(e) => setCurrentAgent({ ...currentAgent, name: e.target.value })}
                className="bg-transparent font-bold outline-none text-sm" 
              />
              <input 
                type="text" 
                value={currentAgent?.description || ''} 
                onChange={(e) => setCurrentAgent({ ...currentAgent, description: e.target.value })}
                className="bg-transparent text-xs text-muted-foreground outline-none" 
              />
            </div>
          </div>
          <button onClick={handleSave} className="flex items-center gap-2 bg-primary text-primary-foreground px-4 py-2 rounded-lg text-xs font-semibold shadow hover:opacity-90 transition-opacity">
            <Save className="w-4 h-4" /> Save Agent
          </button>
        </div>
        <div className="flex-1 w-full h-full relative">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            fitView
          >
            <Controls />
            <MiniMap />
            <Background gap={12} size={1} />
          </ReactFlow>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 overflow-auto h-full bg-background text-foreground">
      <div className="flex justify-between items-center mb-6">
        <div>
          <h2 className="text-2xl font-bold">AI Agents</h2>
          <p className="text-sm text-muted-foreground mt-1">Manage and visually build Eino-powered AI agents.</p>
        </div>
        <button onClick={handleCreateNew} className="flex items-center gap-2 bg-primary text-primary-foreground px-4 py-2 rounded-lg text-sm font-semibold shadow hover:opacity-90 transition-opacity">
          <Plus className="w-4 h-4" /> Create New Agent
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {agents.map((agent) => (
          <div key={agent.id} className="bg-card border border-border rounded-xl p-5 shadow-sm group hover:border-primary/50 transition-colors flex flex-col h-40">
            <div className="flex justify-between items-start mb-2">
              <h3 className="font-bold text-lg truncate">{agent.name}</h3>
              <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                <button onClick={() => handleEdit(agent)} className="p-1.5 hover:bg-accent rounded-md text-muted-foreground hover:text-primary transition-colors">
                  <Edit2 className="w-4 h-4" />
                </button>
                <button onClick={() => handleDelete(agent.id)} className="p-1.5 hover:bg-destructive/10 rounded-md text-muted-foreground hover:text-destructive transition-colors">
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
            <p className="text-sm text-muted-foreground line-clamp-2 flex-1">{agent.description}</p>
            <div className="mt-4 pt-4 border-t border-border flex justify-between items-center text-xs font-mono text-muted-foreground">
              <span>ID: {agent.id.substring(0, 8)}...</span>
              <span className="bg-primary/10 text-primary px-2 py-0.5 rounded font-bold">EINO</span>
            </div>
          </div>
        ))}
        {agents.length === 0 && (
          <div className="col-span-full py-20 flex flex-col items-center justify-center text-muted-foreground italic border-2 border-dashed border-border rounded-xl">
            <p>No agents defined yet. Create one to get started.</p>
          </div>
        )}
      </div>
    </div>
  );
};
