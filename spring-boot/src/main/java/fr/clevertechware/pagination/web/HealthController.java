package fr.clevertechware.pagination.web;

import fr.clevertechware.pagination.service.TransactionService;
import fr.clevertechware.pagination.web.dto.HealthResponse;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class HealthController {

    private final TransactionService service;

    public HealthController(TransactionService service) {
        this.service = service;
    }

    @GetMapping("/healthz")
    public ResponseEntity<HealthResponse> health() {
        return service.isDatabaseReachable()
                ? ResponseEntity.ok(new HealthResponse("ok"))
                : ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE).body(new HealthResponse("down"));
    }
}
