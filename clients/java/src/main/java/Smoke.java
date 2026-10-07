import gophergraph.remote.v1.GraphServiceGrpc;
import gophergraph.remote.v1.Service;
import gophergraph.ggpb.v1.Result;
import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import java.util.Iterator;
import java.util.concurrent.TimeUnit;

/** Local plaintext interoperability smoke, not a general Java client platform. */
public final class Smoke {
  static void consume(Iterator<Service.EncodedBatch> stream) throws Exception {
    boolean header = false, end = false;
    long batches = 0;
    while (stream.hasNext()) { // Exhaustion also checks the final RPC status.
      if (end) throw new IllegalStateException("batch after End");
      Result.Batch batch = Result.Batch.parseFrom(stream.next().getGgpb());
      if (!header) {
        if (!batch.hasHeader() || batch.getHeader().getVersion() != 1) throw new IllegalStateException("header/version");
        header = true;
        System.out.println("query=" + batch.getHeader().getQuery().getName() + " nodes=" + batch.getHeader().getNodes() + " edges=" + batch.getHeader().getEdges());
      } else if (batch.hasEnd()) end = true;
      else if (!batch.hasRecords()) throw new IllegalStateException("unexpected batch");
      batches++;
    }
    if (!header || !end) throw new IllegalStateException("incomplete stream");
    System.out.println("complete batches=" + batches);
  }
  public static void main(String[] args) throws Exception {
    if (args.length != 3) throw new IllegalArgumentException("host port external-id");
    ManagedChannel channel = ManagedChannelBuilder.forAddress(args[0], Integer.parseInt(args[1])).usePlaintext().maxInboundMessageSize((4 << 20) + 16).build();
    try {
      var stub = GraphServiceGrpc.newBlockingStub(channel).withDeadlineAfter(30, TimeUnit.SECONDS);
      var installed = stub.listWasmQueries(Service.ListWasmQueriesRequest.getDefaultInstance());
      if (installed.getQueriesCount() != 3) throw new IllegalStateException("registry");
      consume(stub.territory(Service.TerritoryRequest.newBuilder().setNode(args[2]).build()));
      consume(stub.runWasm(Service.RunWasmRequest.newBuilder().setQueryName("shared-targets").addArgs(args[2]).addArgs(args[2]).build()));
    } finally { channel.shutdownNow().awaitTermination(5, TimeUnit.SECONDS); }
  }
}
