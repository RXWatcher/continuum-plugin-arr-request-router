import { Link } from "react-router";
import RegistryTable from "../components/RegistryTable";

export default function RegistryListPage() {
  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold">Registered *arrs</h2>
        <Link
          to="/registry/new"
          className="px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm hover:opacity-90 transition-opacity"
        >
          Add registered *arr
        </Link>
      </div>
      <RegistryTable />
    </div>
  );
}
