package fr.clevertechware.pagination.web;

import fr.clevertechware.pagination.domain.ExportQuery;
import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.OffsetQuery;
import fr.clevertechware.pagination.service.TransactionService;
import fr.clevertechware.pagination.web.dto.CountEstimateResponse;
import fr.clevertechware.pagination.web.dto.ExportPageResponse;
import fr.clevertechware.pagination.web.dto.KeysetPageResponse;
import fr.clevertechware.pagination.web.dto.OffsetPageResponse;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/v1/transactions")
public class TransactionController {

    private static final int DEFAULT_LIMIT = 20;
    private static final int MAX_LIMIT = 100;
    private static final int DEFAULT_EXPORT_LIMIT = 1000;
    private static final int MAX_EXPORT_LIMIT = 5000;

    private final TransactionService service;

    public TransactionController(TransactionService service) {
        this.service = service;
    }

    @GetMapping
    public KeysetPageResponse list(
            @RequestParam(name = "account_id", required = false) String accountId,
            @RequestParam(name = "status", required = false) String status,
            @RequestParam(name = "sort", required = false) String sort,
            @RequestParam(name = "limit", required = false) String limit,
            @RequestParam(name = "cursor", required = false) String cursor) {

        ListQuery query = new ListQuery(
                QueryParameters.accountId(accountId),
                QueryParameters.status(status),
                QueryParameters.sort(sort),
                QueryParameters.limit(limit, DEFAULT_LIMIT, MAX_LIMIT),
                QueryParameters.cursor(cursor));

        return KeysetPageResponse.from(service.list(query));
    }

    /** The counter-example, kept so the two can be measured side by side. Not the recommendation. */
    @GetMapping("/offset")
    public OffsetPageResponse listByOffset(
            @RequestParam(name = "account_id", required = false) String accountId,
            @RequestParam(name = "page", required = false) String page,
            @RequestParam(name = "size", required = false) String size) {

        OffsetQuery query = new OffsetQuery(
                QueryParameters.accountId(accountId),
                QueryParameters.page(page),
                QueryParameters.size(size, DEFAULT_LIMIT, MAX_LIMIT));

        return OffsetPageResponse.from(service.listByOffset(query));
    }

    /** Full walk over an immutable key: no tie-breaker to manage, every existing row seen once. */
    @GetMapping("/export")
    public ExportPageResponse export(
            @RequestParam(name = "after_id", required = false) String afterId,
            @RequestParam(name = "limit", required = false) String limit) {

        ExportQuery query = new ExportQuery(
                QueryParameters.afterId(afterId),
                QueryParameters.limit(limit, DEFAULT_EXPORT_LIMIT, MAX_EXPORT_LIMIT));

        return ExportPageResponse.from(service.export(query));
    }

    @GetMapping("/count-estimate")
    public CountEstimateResponse countEstimate() {
        return new CountEstimateResponse(service.estimatedRowCount(), false);
    }
}
