package fr.clevertechware.pagination.web;

import fr.clevertechware.pagination.domain.ListQuery;
import fr.clevertechware.pagination.domain.OffsetQuery;
import fr.clevertechware.pagination.service.TransactionService;
import fr.clevertechware.pagination.web.dto.CountEstimateResponse;
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

    @GetMapping("/count-estimate")
    public CountEstimateResponse countEstimate() {
        return new CountEstimateResponse(service.estimatedRowCount(), false);
    }
}
