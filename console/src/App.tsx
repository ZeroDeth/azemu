import { BrowserRouter, Routes, Route, useParams } from 'react-router-dom';
import { TopBar } from './components/TopBar';
import { CockpitView } from './views/CockpitView';
import { PortalView } from './views/PortalView';
import { ExplorerView } from './views/ExplorerView';
import { HealthView } from './views/HealthView';
import { RequestLogView } from './views/RequestLogView';
import { StateStoreView } from './views/StateStoreView';
import { ServiceView, SERVICE_VIEWS } from './views/ServiceView';

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
            {Object.entries(SERVICE_VIEWS).map(([path, service]) => (
              <Route
                key={path}
                path={`/${path}`}
                element={<ServiceView service={service} />}
              />
            ))}
            {/* Namespaced: the console proxy reserves /health and /api/*, so a
                bare /health route would never reach the SPA. */}
            <Route path="/emulator/health" element={<HealthView />} />
            <Route path="/emulator/request-log" element={<RequestLogView />} />
            <Route path="/emulator/state-store" element={<StateStoreView />} />
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
