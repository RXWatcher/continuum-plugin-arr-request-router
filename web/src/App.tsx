import { Routes, Route, Navigate } from "react-router-dom";
import RegistryListPage from "./pages/RegistryListPage";
import RegistryEditorPage from "./pages/RegistryEditorPage";
import RequestsQueuePage from "./pages/RequestsQueuePage";

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<RegistryListPage />} />
      <Route path="/registry/new" element={<RegistryEditorPage />} />
      <Route path="/registry/:id" element={<RegistryEditorPage />} />
      <Route path="/queue" element={<RequestsQueuePage />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
