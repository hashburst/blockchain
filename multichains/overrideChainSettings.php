<?php
// Configurazione percorso del file
$directory = "/home/<username>/public_html/blockchain/ledger/wallets/";

// Funzione per rilevare la modifica del file
function checkFileModification($filePath) {
    static $lastModifiedTime = [];

    if (!file_exists($filePath)) {
        return false;
    }

    $currentModifiedTime = filemtime($filePath);

    // Controllo se è stato modificato
    if (!isset($lastModifiedTime[$filePath]) || $currentModifiedTime != $lastModifiedTime[$filePath]) {
        $lastModifiedTime[$filePath] = $currentModifiedTime;
        return true; // Il file è stato modificato
    }

    return false; // Nessuna modifica
}

// Funzione per inviare i wallet alle API della Mining Pool
function sendToMiningPoolAPI($blockIdSignature, $wallets) {
    $apiUrl = "https://examplepool.com/api/update-wallets"; // URL delle API della mining pool

    $postData = [
        'blockIdSignature' => $blockIdSignature,
        'wallets' => $wallets,
    ];

    // Configurazione della richiesta POST
    $ch = curl_init();
    curl_setopt($ch, CURLOPT_URL, $apiUrl);
    curl_setopt($ch, CURLOPT_POST, true);
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($postData));
    curl_setopt($ch, CURLOPT_HTTPHEADER, [
        'Content-Type: application/json',
        'Authorization: Bearer YOUR_API_KEY_HERE' // Inserisci il token API
    ]);

    $response = curl_exec($ch);

    // Controlla eventuali errori nella richiesta
    if ($response === false) {
        error_log("Errore API: " . curl_error($ch));
    } else {
        $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
        if ($httpCode == 200) {
            echo "Wallet aggiornato con successo per il nodo: $blockIdSignature\n";
        } else {
            error_log("Errore aggiornamento wallet, risposta API: " . $response);
        }
    }

    curl_close($ch);
}

// Funzione principale per processare i file
function processWalletFiles($directory) {
    // Itera sui file presenti nella directory
    foreach (glob($directory . "*") as $filePath) {
        $blockIdSignature = basename($filePath);

        // Verifica se il file è stato modificato
        if (checkFileModification($filePath)) {
            // Leggi il contenuto del file
            $fileContent = file_get_contents($filePath);
            if ($fileContent !== false) {
                $wallets = parseWallets($fileContent);
                sendToMiningPoolAPI($blockIdSignature, $wallets);
            } else {
                error_log("Impossibile leggere il file: $filePath");
            }
        }
    }
}

// Funzione per analizzare il contenuto del file dei wallet
function parseWallets($fileContent) {
    $wallets = [];
    $lines = explode("\n", trim($fileContent));

    foreach ($lines as $line) {
        if (strpos($line, ":") !== false) {
            list($currency, $wallet) = explode(":", $line);
            $wallets[] = [
                'currency' => trim($currency),
                'wallet' => trim($wallet),
            ];
        }
    }

    return $wallets;
}

// Esecuzione dello script ogni X secondi per controllare modifiche
while (true) {
    processWalletFiles($directory);
    sleep(300); // Verifica ogni 5 minuti (300 secondi)
}
