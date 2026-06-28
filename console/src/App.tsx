import { BrowserRouter, Routes, Route, useParams } from 'react-router-dom';
import { TopBar } from './components/TopBar';
import { CockpitView } from './views/CockpitView';
import { PortalView } from './views/PortalView';
import { ExplorerView } from './views/ExplorerView';

export default function App() {
  return (
    <BrowserRouter>
      <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
        <TopBar />
        <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
          <Routes>
            <Route path="/" element={<CockpitView />} />
            <Route path="/resource-groups/:name" element={<PortalViewWrapper />} />
            <Route path="/explorer" element={<ExplorerView />} />
          </Routes>
        </div>
      </div>
    </BrowserRouter>
  );
}

function PortalViewWrapper() {
  const { name } = useParams();
  return <PortalView resourceGroupName={name} />;
}
