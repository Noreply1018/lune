import SectionHeading from "@/components/SectionHeading";

import ExportCard from "./ExportCard";
import ImportCard from "./ImportCard";

type ConfigTransferSectionProps = {
  onImported: () => Promise<void> | void;
};

export default function ConfigTransferSection({
  onImported,
}: ConfigTransferSectionProps) {
  return (
    <section className="surface-section px-5 py-5 sm:px-6">
      <SectionHeading
        title="Configuration Transfer"
        description="导出当前配置快照并从文件恢复。导入前可先预览具体会写入什么。"
      />
      <div className="mt-5 grid gap-4 lg:grid-cols-2 lg:items-stretch">
        <ExportCard />
        <ImportCard onImported={onImported} />
      </div>
    </section>
  );
}
