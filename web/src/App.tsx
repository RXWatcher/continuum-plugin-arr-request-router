import { Routes, Route, Navigate } from "react-router";
import Layout from "./components/Layout";
import RegistryListPage from "./pages/RegistryListPage";
import RegistryEditorPage from "./pages/RegistryEditorPage";
import RequestsQueuePage from "./pages/RequestsQueuePage";
import { Toaster } from "./components/ui/sonner";

export default function App() {
  return (
    <>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<RegistryListPage />} />
          <Route path="/registry/new" element={<RegistryEditorPage />} />
          <Route path="/registry/:id" element={<RegistryEditorPage />} />
          <Route path="/queue" element={<RequestsQueuePage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
      <Toaster />
    </>
  );
}
